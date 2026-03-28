// cmd/import-pinyin/main.go — Import pinyin audio files into the trainer DB.
//
// Scans a directory of MP3 files named like "ba1.mp3" and populates the
// pinyin_sounds + pinyin_progress tables. Optionally copies the MP3 files
// to the pinyin audio directory.
//
// Usage:
//
//	go run ./cmd/import-pinyin [-db data/vocab.db] [-source ./mp3] [-audio-dir data/pinyin-audio] [-dry-run]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	vocabdb "vocabulary_trainer/db"
	"vocabulary_trainer/models"

	_ "modernc.org/sqlite"
)

// Known pinyin initials in order from longest to shortest for greedy matching.
var initials = []string{
	"zh", "ch", "sh",
	"b", "p", "m", "f",
	"d", "t", "n", "l",
	"g", "k", "h",
	"j", "q", "x",
	"z", "c", "s",
	"r", "y", "w",
}

// tagGroups maps initials to their consonant group tag.
var tagGroups = map[string]string{
	"b": "b_p_m_f", "p": "b_p_m_f", "m": "b_p_m_f", "f": "b_p_m_f",
	"d": "d_t_n_l", "t": "d_t_n_l", "n": "d_t_n_l", "l": "d_t_n_l",
	"g": "g_k_h", "k": "g_k_h", "h": "g_k_h",
	"j": "j_q_x", "q": "j_q_x", "x": "j_q_x",
	"zh": "zh_ch_sh_r", "ch": "zh_ch_sh_r", "sh": "zh_ch_sh_r", "r": "zh_ch_sh_r",
	"z": "z_c_s", "c": "z_c_s", "s": "z_c_s",
	"y": "y_w", "w": "y_w",
}

func splitInitialFinal(syllable string) (string, string) {
	s := strings.ToLower(syllable)
	for _, ini := range initials {
		if strings.HasPrefix(s, ini) {
			return ini, s[len(ini):]
		}
	}
	return "", s // pure vowel
}

func parseFilename(name string) (syllable string, tone int, ok bool) {
	// Expect "ba1.mp3"
	name = strings.TrimSuffix(name, ".mp3")
	if len(name) < 2 {
		return "", 0, false
	}
	last := name[len(name)-1]
	if last < '1' || last > '4' {
		return "", 0, false
	}
	tone = int(last - '0')
	syllable = name[:len(name)-1]
	if syllable == "" {
		return "", 0, false
	}
	return syllable, tone, true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	sourceDir := flag.String("source", "mp3", "directory containing pinyin MP3 files")
	audioDir := flag.String("audio-dir", "", "destination directory for audio files (default: sibling of DB)")
	dryRun := flag.Bool("dry-run", false, "parse files but do not insert or copy")
	flag.Parse()

	if *audioDir == "" {
		*audioDir = filepath.Join(filepath.Dir(*dbPath), "pinyin-audio")
	}

	// Scan source directory
	entries, err := os.ReadDir(*sourceDir)
	if err != nil {
		log.Fatalf("read source dir %s: %v", *sourceDir, err)
	}

	type sound struct {
		syllable string
		tone     int
		filename string
		srcPath  string
	}
	var sounds []sound
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mp3") {
			continue
		}
		syllable, tone, ok := parseFilename(e.Name())
		if !ok {
			log.Printf("[SKIP]  %s (cannot parse filename)", e.Name())
			continue
		}
		sounds = append(sounds, sound{
			syllable: syllable,
			tone:     tone,
			filename: e.Name(),
			srcPath:  filepath.Join(*sourceDir, e.Name()),
		})
	}

	log.Printf("Found %d valid pinyin MP3 files in %s", len(sounds), *sourceDir)

	if *dryRun {
		for _, s := range sounds {
			initial, final := splitInitialFinal(s.syllable)
			tag := tagGroups[initial]
			if tag == "" {
				tag = "vowels"
			}
			fmt.Printf("  %s → syllable=%s tone=%d initial=%s final=%s tag=%s\n",
				s.filename, s.syllable, s.tone, initial, final, tag)
		}
		log.Printf("[DRY RUN] Would insert %d sounds", len(sounds))
		return
	}

	// Open DB and migrate
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)", *dbPath)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	if err := vocabdb.Migrate(database); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// Create audio directory
	if err := os.MkdirAll(*audioDir, 0755); err != nil {
		log.Fatalf("create audio dir %s: %v", *audioDir, err)
	}

	// Use a Store for DB operations
	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	var inserted, skipped int
	for _, s := range sounds {
		initial, final := splitInitialFinal(s.syllable)
		tag := tagGroups[initial]
		if tag == "" {
			tag = "vowels"
		}

		ps := models.PinyinSound{
			Initial:  initial,
			Final:    final,
			Tone:     s.tone,
			Syllable: s.syllable,
			Filename: s.filename,
			Tag:      tag,
		}

		id, err := store.InsertPinyinSound(context.Background(), ps)
		if err != nil {
			log.Printf("[ERROR] %s: %v", s.filename, err)
			continue
		}

		// Copy MP3 to audio directory
		dstPath := filepath.Join(*audioDir, s.filename)
		if _, err := os.Stat(dstPath); os.IsNotExist(err) {
			if err := copyFile(s.srcPath, dstPath); err != nil {
				log.Printf("[ERROR] copy %s: %v", s.filename, err)
				continue
			}
		}

		if id > 0 {
			inserted++
		} else {
			skipped++
		}
	}

	log.Printf("Done: %d inserted, %d already existed", inserted, skipped)
}

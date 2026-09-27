// cmd/classify-topics/main.go — Sort vocabulary into topic word lists with Claude.
//
// Candidates are every library word tagged hsk2-N / hsk3-N plus the top -freq
// zh words of word_frequency_lang that have a CC-CEDICT entry (see
// Store.GetTopicCandidates). Claude assigns each word 0–2 topics from the fixed
// list below. Results are appended to <out>/<topic>.csv as "zh,en" rows (en is
// the first CEDICT gloss); words with no topic go to <out>/_none.csv.
//
// The tool is resumable: every zh already present in any CSV under <out> is
// skipped, so a rerun after an error only classifies what is left.
//
// Usage:
//
//	ANTHROPIC_API_KEY=... go run ./cmd/classify-topics [-db data/vocab.db] [-out data/topics] [-dry-run]
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	vocabdb "vocabulary_trainer/db"
	"vocabulary_trainer/llm"
	"vocabulary_trainer/models"
)

// topics maps each topic slug (the CSV file name) to the hint given to Claude.
var topics = map[string]string{
	"greetings":  "greetings, courtesy, basic social phrases",
	"numbers":    "numbers, counting, measure words, quantities",
	"time":       "time, dates, days, months, clock, frequency",
	"family":     "family members, relatives, people and relationships",
	"food":       "food, drinks, cooking, restaurants, flavours",
	"shopping":   "shopping, money, prices, payment, stores",
	"body":       "body parts and bodily actions",
	"colors":     "colours, shapes, appearance",
	"directions": "directions, positions, places in town, getting around",
	"weather":    "weather, seasons, climate",
	"home":       "home, rooms, furniture, household items, chores, renting",
	"travel":     "travel, transport, hotels, tickets, luggage, sightseeing",
	"health":     "health, illness, symptoms, doctor, hospital, medicine",
	"hobbies":    "hobbies, leisure, games, free-time activities",
	"sports":     "sports, exercise, competitions",
	"work":       "work, jobs, office, professions, careers",
	"school":     "school, study, education, exams, subjects",
	"feelings":   "feelings, emotions, personality traits",
	"clothing":   "clothing, accessories, wearing and dressing",
	"technology": "phone, internet, computers, apps, digital life",
	"business":   "business, finance, economy, trade, banking",
	"politics":   "politics, government, society, news, international affairs",
	"science":    "science, research, environment, energy",
	"law":        "law, crime, police, bureaucracy, documents",
	"culture":    "culture, history, traditions, festivals, religion",
	"nature":     "nature, landscape, animals, plants",
	"gardening":  "gardening, growing plants, garden tools, farming",
	"arts":       "arts, music, film, literature, media",
}

const noneFile = "_none"

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	outDir := flag.String("out", "data/topics", "directory for the <topic>.csv files")
	maxFreq := flag.Int("freq", 8000, "also classify non-HSK words up to this zh frequency rank")
	chunk := flag.Int("chunk", 100, "words per Claude request")
	model := flag.String("model", "claude-opus-5", "Claude model ID")
	effort := flag.String("effort", "low", "Claude effort level (low|medium|high)")
	dryRun := flag.Bool("dry-run", false, "print how many words would be classified, without calling Claude")
	flag.Parse()

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	cands, err := store.GetTopicCandidates(ctx, *maxFreq)
	if err != nil {
		log.Fatalf("load candidates: %v", err)
	}
	done, err := loadDone(*outDir)
	if err != nil {
		log.Fatalf("read %s: %v", *outDir, err)
	}
	var todo []models.TopicCandidate
	for _, c := range cands {
		if !done[c.Zh] {
			todo = append(todo, c)
		}
	}
	fmt.Printf("%d candidate word(s), %d already classified, %d to do\n", len(cands), len(cands)-len(todo), len(todo))
	if *dryRun || len(todo) == 0 {
		return
	}

	client := llm.NewClientFromConfig("anthropic", os.Getenv("ANTHROPIC_API_KEY"), "")
	if client == nil {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create %s: %v", *outDir, err)
	}

	system := systemPrompt()
	var classified, missing int
	for i := 0; i < len(todo); i += *chunk {
		batch := todo[i:min(i+*chunk, len(todo))]
		text, err := client.Generate(ctx, llm.Request{System: system, User: userPrompt(batch), Model: *model, Effort: *effort})
		if err != nil {
			// ponytail: no retry loop — rerunning the tool resumes where it stopped.
			log.Fatalf("classify words %d-%d: %v", i+1, i+len(batch), err)
		}
		res := parseResponse(text, batch)
		if err := writeResults(*outDir, batch, res); err != nil {
			log.Fatalf("write results: %v", err)
		}
		classified += len(res)
		missing += len(batch) - len(res)
		fmt.Printf("  %d/%d classified\n", i+len(batch), len(todo))
	}
	fmt.Printf("\nDone. classified=%d  missing=%d\n", classified, missing)
	if missing > 0 {
		fmt.Println("Rerun the tool to classify the missing words.")
	}
}

func systemPrompt() string {
	var b strings.Builder
	b.WriteString("You sort Chinese vocabulary into topic word lists for learners of Chinese.\n\nTopics:\n")
	for _, slug := range slices.Sorted(maps.Keys(topics)) {
		fmt.Fprintf(&b, "%s: %s\n", slug, topics[slug])
	}
	b.WriteString(`
Each input line is a Chinese word, a tab, and its first dictionary gloss.
For every word, pick the topics a learner studying that topic should meet it in: usually 1, at most 2.
Use "-" when no topic fits well, for example grammar words, pronouns, generic verbs and adjectives, or rare words.
Answer with exactly one line per input word, in the same order: the Chinese word, a tab, then comma-separated topic slugs or "-".
Output nothing else.`)
	return b.String()
}

func userPrompt(batch []models.TopicCandidate) string {
	var b strings.Builder
	for _, c := range batch {
		fmt.Fprintf(&b, "%s\t%s\n", c.Zh, c.En)
	}
	return b.String()
}

// parseResponse maps each word of batch that Claude answered to its topics
// (empty for "-"). Unknown topic slugs and words not in batch are ignored;
// words Claude skipped are absent from the map and get classified on a rerun.
func parseResponse(text string, batch []models.TopicCandidate) map[string][]string {
	inBatch := map[string]bool{}
	for _, c := range batch {
		inBatch[c.Zh] = true
	}
	res := map[string][]string{}
	for _, line := range strings.Split(text, "\n") {
		zh, rest, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || !inBatch[zh] {
			continue
		}
		picked := []string{}
		for _, slug := range strings.Split(rest, ",") {
			slug = strings.TrimSpace(slug)
			if _, known := topics[slug]; known {
				picked = append(picked, slug)
			}
		}
		res[zh] = picked
	}
	return res
}

// writeResults appends each answered word of batch to its topic CSVs, or to
// _none.csv when it has no topic. New files start with a "zh,en" header.
func writeResults(dir string, batch []models.TopicCandidate, res map[string][]string) error {
	rows := map[string][][]string{}
	for _, c := range batch {
		picked, ok := res[c.Zh]
		if !ok {
			continue
		}
		if len(picked) == 0 {
			picked = []string{noneFile}
		}
		for _, slug := range picked {
			rows[slug] = append(rows[slug], []string{c.Zh, c.En})
		}
	}
	for slug, recs := range rows {
		path := filepath.Join(dir, slug+".csv")
		_, statErr := os.Stat(path)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		w := csv.NewWriter(f)
		if errors.Is(statErr, fs.ErrNotExist) {
			w.Write([]string{"zh", "en"})
		}
		w.WriteAll(recs)
		if err := errors.Join(w.Error(), f.Close()); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// loadDone returns every zh already written to a CSV in dir.
func loadDone(dir string) (map[string]bool, error) {
	done := map[string]bool{}
	paths, err := filepath.Glob(filepath.Join(dir, "*.csv"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		recs, err := csv.NewReader(f).ReadAll()
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		for i, r := range recs {
			if i > 0 && len(r) > 0 {
				done[r[0]] = true
			}
		}
	}
	return done, nil
}

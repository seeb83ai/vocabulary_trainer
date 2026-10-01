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
//	go run ./cmd/classify-topics -claude-cli claude   # use the Claude Code subscription login instead
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

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
	workers := flag.Int("workers", 4, "Claude requests to run at the same time")
	claudeBin := flag.String("claude-cli", "", `run requests through this Claude Code binary (e.g. "claude") with your subscription login instead of ANTHROPIC_API_KEY`)
	flag.Parse()

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	// Ctrl-C stops new requests but lets running ones finish and be written.
	// A second Ctrl-C quits at once (the running requests are then redone next run).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
		fmt.Println("\nStopping: finishing the requests in flight. Press Ctrl-C again to quit now.")
	}()
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

	generate := func(system, user string) (string, error) {
		return claudeCLI(context.Background(), *claudeBin, *model, *effort, system, user)
	}
	if *claudeBin == "" {
		client := llm.NewClientFromConfig("anthropic", os.Getenv("ANTHROPIC_API_KEY"), "")
		if client == nil {
			log.Fatal("ANTHROPIC_API_KEY environment variable is required (or use -claude-cli claude)")
		}
		generate = func(system, user string) (string, error) {
			return client.Generate(context.Background(), llm.Request{System: system, User: user, Model: *model, Effort: *effort})
		}
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create %s: %v", *outDir, err)
	}

	system := systemPrompt()
	classified, missing, err := classifyAll(ctx, todo, *chunk, *workers, *outDir, func(user string) (string, error) {
		return generate(system, user)
	})
	fmt.Printf("\nclassified=%d  missing=%d\n", classified, missing)
	if err != nil {
		// ponytail: no retry loop — rerunning the tool resumes where it stopped.
		log.Fatalf("stopped: %v\nRerun the tool to continue.", err)
	}
	if missing > 0 {
		fmt.Println("Rerun the tool to classify the missing words.")
	}
}

// classifyAll sends todo to generate in chunks, running up to workers chunks
// at once, and appends each answer to the CSVs as soon as it arrives. After
// the first error, or when ctx is cancelled (Ctrl-C), no new chunks start;
// chunks already in flight still finish and get written.
func classifyAll(ctx context.Context, todo []models.TopicCandidate, chunk, workers int, outDir string, generate func(user string) (string, error)) (classified, missing int, err error) {
	batches := make(chan []models.TopicCandidate)
	var mu sync.Mutex // guards the CSV files, the counters and err
	stopped := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return err != nil || ctx.Err() != nil
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batches {
				if stopped() {
					continue
				}
				text, genErr := generate(userPrompt(batch))
				mu.Lock()
				if genErr == nil {
					res := parseResponse(text, batch)
					genErr = writeResults(outDir, batch, res)
					if genErr == nil {
						classified += len(res)
						missing += len(batch) - len(res)
						fmt.Printf("  %d/%d words done\n", classified+missing, len(todo))
					}
				}
				if genErr != nil && err == nil {
					err = genErr
				}
				mu.Unlock()
			}
		}()
	}

dispatch:
	for i := 0; i < len(todo) && !stopped(); i += chunk {
		select {
		case batches <- todo[i:min(i+chunk, len(todo))]:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(batches)
	wg.Wait()
	if err == nil {
		err = ctx.Err()
	}
	return classified, missing, err
}

// claudeCLI sends one request through Claude Code in print mode, so it runs on
// the logged-in Claude subscription. Tools are disabled and the system prompt
// replaces Claude Code's own, so the answer is plain text like the API's.
func claudeCLI(ctx context.Context, bin, model, effort, system, user string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, "-p",
		"--model", model,
		"--effort", effort,
		"--system-prompt", system,
		"--tools", "",
		"--no-session-persistence",
		"--output-format", "text")
	cmd.Stdin = strings.NewReader(user)
	// Own process group, so a Ctrl-C in the terminal doesn't kill a request in flight.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", bin, err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
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

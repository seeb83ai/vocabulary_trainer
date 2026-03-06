package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed frontend
var frontendFS embed.FS

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/vocab.db"
	}

	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create data directory %s: %v", dir, err)
		}
	}

	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	// TTS audio handler — always enabled.
	// AUDIO_DIR defaults to a sibling of the DB file.
	audioDir := os.Getenv("AUDIO_DIR")
	if audioDir == "" {
		audioDir = filepath.Join(filepath.Dir(dbPath), "audio")
	}
	audioH := &handlers.AudioHandler{
		Store:    store,
		AudioDir: audioDir,
	}
	log.Printf("TTS enabled: audio=%s", audioDir)

	authH, err := handlers.NewAuthHandler(os.Getenv("AUTH_USER"), os.Getenv("AUTH_PASSWORD"))
	if err != nil {
		log.Fatalf("Failed to initialise auth: %v", err)
	}
	if authH != nil {
		log.Printf("Auth enabled: user=%s", os.Getenv("AUTH_USER"))
	}

	var translateH *handlers.TranslateHandler
	if key := os.Getenv("DEEPL_API_KEY"); key != "" {
		lang := os.Getenv("DEEPL_TARGET_LANGUAGE")
		if lang == "" {
			lang = "EN"
		}
		translateH = &handlers.TranslateHandler{
			APIKey:     key,
			TargetLang: strings.ToUpper(lang),
		}
		log.Printf("DeepL translation enabled: target=%s", strings.ToUpper(lang))
	}

	maxNewWords := 5
	if v := os.Getenv("MAX_NEW_WORDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxNewWords = n
		}
	}
	log.Printf("Daily new-word cap: %d (set MAX_NEW_WORDS to change)", maxNewWords)

	wordsH := &handlers.WordsHandler{Store: store, Audio: audioH}
	quizH := &handlers.QuizHandler{Store: store, MaxNewPerDay: maxNewWords}
	mismatchH := &handlers.MismatchesHandler{Store: store}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	if authH != nil {
		r.Use(authH.Middleware)
	}

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/auth/status", handlers.AuthStatus(authH))
		if authH != nil {
			r.Post("/login", authH.Login)
			r.Post("/logout", authH.Logout)
		}
		r.Get("/quiz/next", quizH.Next)
		r.Post("/quiz/answer", quizH.Answer)
		r.Post("/quiz/skip", quizH.Skip)
		r.Post("/quiz/acknowledge", quizH.Acknowledge)
		r.Get("/quiz/stats", quizH.Stats)
		r.Get("/quiz/daily-stats", quizH.DailyStats)
		r.Route("/words", func(r chi.Router) {
			r.Get("/", wordsH.List)
			r.Post("/", wordsH.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", wordsH.GetByID)
				r.Put("/", wordsH.Update)
				r.Delete("/", wordsH.Delete)
				r.Post("/translations", wordsH.AddTranslation)
				r.Post("/review", wordsH.MarkReview)
			})
		})
		r.Get("/tags", wordsH.ListTags)
		r.Get("/audio/{id}", audioH.ServeAudio)
		r.Get("/mismatches", mismatchH.List)
		r.Get("/config", handlers.Config(translateH != nil))
		if translateH != nil {
			r.Post("/translate", translateH.Translate)
		}
	})

	// Static frontend files
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("Failed to create sub FS: %v", err)
	}

	fileServer := http.FileServer(http.FS(sub))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		serveFileFromFS(w, r, sub, "index.html")
	})
	r.Get("/vocab", func(w http.ResponseWriter, r *http.Request) {
		serveFileFromFS(w, r, sub, "vocab.html")
	})
	r.Get("/mismatches", func(w http.ResponseWriter, r *http.Request) {
		serveFileFromFS(w, r, sub, "mismatches.html")
	})
	r.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
		serveFileFromFS(w, r, sub, "stats.html")
	})
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		serveFileFromFS(w, r, sub, "login.html")
	})
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("Vocabulary Trainer listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func serveFileFromFS(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	f, err := fsys.Open(name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	f.Close()
	http.ServeFileFS(w, r, fsys, name)
}

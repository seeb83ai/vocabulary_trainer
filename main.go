package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
		dbPath = "/data/vocab.db"
	}

	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	// TTS audio handler — enabled when TTS_SCRIPT env var points to generate.py.
	// AUDIO_DIR defaults to a sibling of the DB file; VENV_PYTHON defaults to python3.
	var audioH *handlers.AudioHandler
	ttsScript := os.Getenv("TTS_SCRIPT")
	if ttsScript != "" {
		audioDir := os.Getenv("AUDIO_DIR")
		if audioDir == "" {
			audioDir = filepath.Join(filepath.Dir(dbPath), "audio")
		}
		audioH = &handlers.AudioHandler{
			Store:      store,
			AudioDir:   audioDir,
			TTSScript:  ttsScript,
			VenvPython: os.Getenv("VENV_PYTHON"),
		}
		log.Printf("TTS enabled: script=%s audio=%s", ttsScript, audioDir)
	}

	wordsH := &handlers.WordsHandler{Store: store, Audio: audioH}
	quizH := &handlers.QuizHandler{Store: store}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/quiz/next", quizH.Next)
		r.Post("/quiz/answer", quizH.Answer)
		r.Get("/quiz/stats", quizH.Stats)
		r.Route("/words", func(r chi.Router) {
			r.Get("/", wordsH.List)
			r.Post("/", wordsH.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", wordsH.GetByID)
				r.Put("/", wordsH.Update)
				r.Delete("/", wordsH.Delete)
				r.Post("/translations", wordsH.AddTranslation)
			})
		})
		if audioH != nil {
			r.Get("/audio/{id}", audioH.ServeAudio)
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
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	})

	addr := ":8080"
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

package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/email"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/llm"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed frontend
var frontendFS embed.FS

type PageData struct {
	Title       string
	ActiveNav   string
	ExtraHead   template.HTML
	PageScripts []string
}

var templateCache map[string]*template.Template

func initTemplates(fsys fs.FS) {
	templateCache = make(map[string]*template.Template)
	pages := []string{"train", "vocab", "stats", "mnemonics", "mismatches", "pinyin", "settings"}
	for _, name := range pages {
		t, err := template.ParseFS(fsys, "layout.html", name+".html")
		if err != nil {
			log.Fatalf("template parse error for %s: %v", name, err)
		}
		templateCache[name] = t
	}
}

func renderTemplate(w http.ResponseWriter, name string, data PageData) {
	t, ok := templateCache[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("template execute error %s: %v", name, err)
	}
}

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

	emailSender := email.NewSenderFromEnv()
	if emailSender != nil {
		log.Printf("Email enabled: SMTP configured")
	} else {
		log.Printf("Email disabled: SMTP not configured (accounts auto-verified)")
	}

	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		port0 := os.Getenv("PORT")
		if port0 == "" {
			port0 = "8080"
		}
		appURL = "http://localhost:" + port0
	}
	log.Printf("App URL: %s", appURL)

	sessionSecret := os.Getenv("SESSION_SECRET")
	authH, err := handlers.NewAuthHandler(store, emailSender, appURL, sessionSecret)
	if err != nil {
		log.Fatalf("Failed to initialise auth: %v", err)
	}

	translateH := &handlers.TranslateHandler{Store: store}
	if key := os.Getenv("DEEPL_API_KEY"); key != "" {
		lang := os.Getenv("DEEPL_TARGET_LANGUAGE")
		if lang == "" {
			lang = "EN"
		}
		translateH.APIKey = key
		translateH.TargetLang = strings.ToUpper(lang)
		log.Printf("DeepL translation enabled: target=%s", strings.ToUpper(lang))
	}

	maxNewWords := 5
	if v := os.Getenv("MAX_NEW_WORDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxNewWords = n
		}
	}
	log.Printf("Daily new-word cap: %d (set MAX_NEW_WORDS to change)", maxNewWords)

	pinyinAudioDir := os.Getenv("PINYIN_AUDIO_DIR")

	pinyinAudioDirs := []string{}
	if extra := os.Getenv("PINYIN_AUDIO_DIRS"); extra != "" {
		for _, d := range strings.Split(extra, ":") {
			if d = strings.TrimSpace(d); d != "" {
				pinyinAudioDirs = append(pinyinAudioDirs, d)
			}
		}
	} else if pinyinAudioDir == "" {
		pinyinAudioDirs = append(pinyinAudioDirs, filepath.Join(filepath.Dir(dbPath), "pinyin-audio"))
	} else {
		pinyinAudioDirs = append(pinyinAudioDirs, pinyinAudioDir)
	}
	log.Printf("Pinyin audio dirs: %v", pinyinAudioDirs)

	wordsH := &handlers.WordsHandler{Store: store, Audio: audioH}
	importH := &handlers.ImportHandler{Store: store}
	tagsH := &handlers.TagsHandler{Store: store}
	quizH := &handlers.QuizHandler{Store: store, MaxNewPerDay: maxNewWords}
	mismatchH := &handlers.MismatchesHandler{Store: store}
	hanziH := &handlers.HanziHandler{Store: store}
	hmmH := &handlers.HMMHandler{Store: store, DeepLAPIKey: translateH.APIKey}
	hmmQuizH := &handlers.HMMQuizHandler{Store: store}
	pinyinQuizH := &handlers.PinyinQuizHandler{Store: store, PinyinAudioDirs: pinyinAudioDirs}

	llmClient := llm.NewClientFromEnv()
	var llmH *handlers.LLMHandler
	if llmClient != nil {
		llmH = &handlers.LLMHandler{Client: llmClient, Store: store}
		log.Printf("LLM enabled: provider=%s", llmClient.Name())
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			next.ServeHTTP(w, r)
		})
	})
	r.Use(authH.Middleware)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/auth/status", handlers.AuthStatus(authH))
		r.Post("/login", authH.Login)
		r.Post("/logout", authH.Logout)
		r.Post("/register", authH.Register)
		r.Get("/verify-email", authH.VerifyEmail)
		r.Get("/me", authH.Me)
		r.Post("/change-password", authH.ChangePassword)
		r.Get("/quiz/next", quizH.Next)
		r.Post("/quiz/answer", quizH.Answer)
		r.Get("/quiz/langs", quizH.Langs)
		r.Post("/quiz/skip", quizH.Skip)
		r.Post("/quiz/acknowledge", quizH.Acknowledge)
		r.Post("/quiz/acknowledge-random", quizH.AcknowledgeRandom)
		r.Post("/quiz/advance", quizH.Advance)
		r.Get("/quiz/stats", quizH.Stats)
		r.Get("/quiz/daily-stats", quizH.DailyStats)
		r.Get("/quiz/word-stats", quizH.WordStats)
		r.Get("/quiz/due-date-distribution", quizH.DueDateDistribution)
		r.Route("/words", func(r chi.Router) {
			r.Get("/", wordsH.List)
			r.Post("/", wordsH.Create)
			r.Get("/export", wordsH.Export)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", wordsH.GetByID)
				r.Put("/", wordsH.Update)
				r.Delete("/", wordsH.Delete)
				r.Post("/translations", wordsH.AddTranslation)
				r.Post("/review", wordsH.MarkReview)
				r.Get("/hmm/context", hmmH.GetSceneContext)
				r.Put("/hmm", hmmH.SaveScene)
				r.Delete("/hmm", hmmH.DeleteScene)
				if llmH != nil {
					r.Post("/hmm/generate-scene", llmH.GenerateScene)
				}
			})
		})
		r.Get("/tags", wordsH.ListTags)
		r.Get("/import/source-tags", importH.SourceTags)
		r.Get("/import/preview", importH.Preview)
		r.Post("/import", importH.Import)
		r.Get("/tags/details", tagsH.Details)
		r.Put("/tags/{name}", tagsH.Update)
		r.Get("/audio/{id}", audioH.ServeAudio)
		r.Get("/mismatches", mismatchH.List)
		r.Get("/hanzi/decompose", hanziH.Decompose)
		r.Post("/pinyin", handlers.Pinyin)
		r.Route("/hmm", func(r chi.Router) {
			r.Get("/actors", hmmH.GetActors)
			r.Put("/actors/{initial}", hmmH.UpdateActor)
			r.Get("/locations", hmmH.GetLocations)
			r.Put("/locations/{final}", hmmH.UpdateLocation)
			r.Get("/tone-rooms", hmmH.GetToneRooms)
			r.Put("/tone-rooms/{tone}", hmmH.UpdateToneRoom)
			r.Get("/props", hmmH.GetProps)
			r.Put("/props", hmmH.UpsertProp)
			r.Delete("/props/{radical}", hmmH.DeleteProp)
		})
		r.Get("/pinyin-quiz/next", pinyinQuizH.Next)
		r.Post("/pinyin-quiz/answer", pinyinQuizH.Answer)
		r.Get("/pinyin-quiz/stats", pinyinQuizH.Stats)
		r.Get("/pinyin-quiz/daily-stats", pinyinQuizH.DailyStats)
		r.Get("/pinyin-quiz/audio/{filename}", pinyinQuizH.ServeAudio)
		r.Get("/pinyin-quiz/tags", pinyinQuizH.ListTags)
		r.Post("/hmm-quiz/answer", hmmQuizH.Answer)
		r.Get("/config", translateH.Config(translateH.APIKey != "", llmH != nil))
		if translateH.APIKey != "" {
			r.Post("/translate", translateH.Translate)
		}
	})

	// Static frontend files
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("Failed to create sub FS: %v", err)
	}

	initTemplates(sub)
	fileServer := http.FileServer(http.FS(sub))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		serveFileFromFS(w, r, sub, "index.html")
	})
	r.Get("/vocab", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "vocab", PageData{
			Title:       "Vocabulary — Vocab Trainer",
			ActiveNav:   "vocab",
			PageScripts: []string{"hmm-builder.js", "vocab.js"},
		})
	})
	r.Get("/mismatches", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "mismatches", PageData{
			Title:       "Mismatches — Vocab Trainer",
			ActiveNav:   "mismatches",
			PageScripts: []string{"mismatches.js"},
		})
	})
	r.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "stats", PageData{
			Title:       "Stats — Vocab Trainer",
			ActiveNav:   "stats",
			ExtraHead:   template.HTML(`<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>`),
			PageScripts: []string{"stats.js"},
		})
	})
	r.Get("/mnemonics", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "mnemonics", PageData{
			Title:       "Mnemonics — Vocab Trainer",
			ActiveNav:   "mnemonics",
			PageScripts: []string{"mnemonics.js"},
		})
	})
	r.Get("/train", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "train", PageData{
			Title:       "Train — Vocab Trainer",
			ActiveNav:   "train",
			PageScripts: []string{"hmm-builder.js", "train.js"},
		})
	})
	r.Get("/settings", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "settings", PageData{
			Title:       "Settings — Vocab Trainer",
			ActiveNav:   "settings",
			PageScripts: []string{"settings.js"},
		})
	})
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	r.Get("/pinyin", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "pinyin", PageData{
			Title:       "Pinyin Listening · Vocab Trainer",
			ActiveNav:   "pinyin",
			PageScripts: []string{"pinyin.js"},
		})
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

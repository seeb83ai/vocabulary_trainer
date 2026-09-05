package main

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
)

// impressumData holds the operator identity shown on the /impressum page.
// German law (TMG §5) requires a public site operated from Germany to name
// a responsible party and a way to contact them — content only the site
// operator can supply accurately, so it comes from the environment rather
// than being hardcoded.
type impressumData struct {
	Name    string
	Address string
	Email   string
	Phone   string // optional — left blank, not placeholder text, when unset
}

// envOrPlaceholder reads an env var, or returns a placeholder that names the
// var to set so a deployer notices the missing legal content immediately
// instead of accidentally publishing a blank Impressum.
func envOrPlaceholder(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return "[Add your details — set " + name + " in .env]"
}

func loadImpressumData() impressumData {
	return impressumData{
		Name:    envOrPlaceholder("IMPRESSUM_NAME"),
		Address: envOrPlaceholder("IMPRESSUM_ADDRESS"),
		Email:   envOrPlaceholder("IMPRESSUM_EMAIL"),
		Phone:   os.Getenv("IMPRESSUM_PHONE"),
	}
}

var impressumTemplate *template.Template

func initImpressumTemplate(fsys fs.FS) {
	t, err := template.ParseFS(fsys, "impressum.html")
	if err != nil {
		log.Fatalf("template parse error for impressum: %v", err)
	}
	impressumTemplate = t
}

func impressumHandler(w http.ResponseWriter, r *http.Request) {
	if impressumTemplate == nil {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := impressumTemplate.Execute(w, loadImpressumData()); err != nil {
		log.Printf("impressum template execute error: %v", err)
	}
}

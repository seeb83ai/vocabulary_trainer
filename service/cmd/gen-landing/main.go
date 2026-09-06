// Command gen-landing builds the feature-teaser grid on the landing page
// (service/frontend/index.html) from the teaser folders in
// service/frontend/landing/teasers/. Each folder holds a teaser.json
// (category/title/summary/imageAlt) plus zero or more screenshot images,
// auto-detected by globbing the folder: zero images renders a text-only
// card, one a standalone image, two or more a cross-fade carousel.
//
// Both the JSON copy and the image list are read here, at build time, and
// baked directly into index.html — the shipped page never fetches a
// manifest or image list over the network for this.
//
// Run via `make generate-landing`, which `make build`, `make restart` and
// `make release` all depend on, so the landing page never goes stale.
package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	teasersDirDefault = "frontend/landing/teasers"
	indexHTMLDefault  = "frontend/index.html"
	startMarker       = "<!-- LANDING-TEASERS:START -->"
	endMarker         = "<!-- LANDING-TEASERS:END -->"

	// imageURLPrefix is the path the embedded file server serves teaser
	// images under — service/frontend is served at "/", so a file at
	// service/frontend/landing/teasers/<slug>/<file> is reachable at
	// /landing/teasers/<slug>/<file>.
	imageURLPrefix = "/landing/teasers"
)

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true,
}

type teaserConfig struct {
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	ImageAlt []string `json:"imageAlt"`
}

func main() {
	teasersDir := teasersDirDefault
	indexHTML := indexHTMLDefault
	if len(os.Args) > 1 {
		teasersDir = os.Args[1]
	}
	if len(os.Args) > 2 {
		indexHTML = os.Args[2]
	}

	teaserHTML, err := renderTeasers(teasersDir)
	if err != nil {
		log.Fatalf("gen-landing: %v", err)
	}

	doc, err := os.ReadFile(indexHTML)
	if err != nil {
		log.Fatalf("gen-landing: read %s: %v", indexHTML, err)
	}

	updated, err := spliceMarkers(string(doc), teaserHTML)
	if err != nil {
		log.Fatalf("gen-landing: %s: %v", indexHTML, err)
	}

	if err := os.WriteFile(indexHTML, []byte(updated), 0o644); err != nil {
		log.Fatalf("gen-landing: write %s: %v", indexHTML, err)
	}
	log.Printf("gen-landing: wrote teaser grid into %s", indexHTML)
}

// renderTeasers reads every teaser folder under dir, sorted by folder name,
// and renders each into an <article> card. A folder must contain a
// teaser.json; a missing one is a build error rather than a silently empty
// card, since a teaser with no copy is never intentional.
func renderTeasers(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read teasers dir %s: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var out strings.Builder
	for _, name := range names {
		card, err := renderTeaser(filepath.Join(dir, name), name)
		if err != nil {
			return "", err
		}
		out.WriteString(card)
		out.WriteString("\n")
	}
	return out.String(), nil
}

func renderTeaser(dirPath, slug string) (string, error) {
	cfgBytes, err := os.ReadFile(filepath.Join(dirPath, "teaser.json"))
	if err != nil {
		return "", fmt.Errorf("teaser folder %q: missing teaser.json: %w", slug, err)
	}
	var cfg teaserConfig
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return "", fmt.Errorf("teaser folder %q: invalid teaser.json: %w", slug, err)
	}
	if cfg.Category == "" || cfg.Title == "" || cfg.Summary == "" {
		return "", fmt.Errorf("teaser folder %q: teaser.json needs category, title and summary", slug)
	}

	images, err := listImages(dirPath)
	if err != nil {
		return "", fmt.Errorf("teaser folder %q: %w", slug, err)
	}

	key := i18nSlug(slug)

	var b strings.Builder
	b.WriteString(`<article class="border border-gray-200 rounded-3xl overflow-hidden bg-white shadow-sm hover:shadow-xl hover:-translate-y-1 transition">` + "\n")
	b.WriteString(renderImageBlock(slug, images, cfg))
	b.WriteString(`  <div class="p-5 pb-6">` + "\n")
	fmt.Fprintf(&b, `    <span class="text-[11px] font-bold tracking-wide uppercase text-blue-600" data-i18n="landing.teaser.%s.category">%s</span>`+"\n", key, html.EscapeString(cfg.Category))
	fmt.Fprintf(&b, `    <h3 class="mt-2 mb-1.5 text-lg font-bold text-gray-900" data-i18n="landing.teaser.%s.title">%s</h3>`+"\n", key, html.EscapeString(cfg.Title))
	fmt.Fprintf(&b, `    <p class="text-sm leading-relaxed text-gray-600" data-i18n="landing.teaser.%s.summary">%s</p>`+"\n", key, html.EscapeString(cfg.Summary))
	b.WriteString(`  </div>` + "\n")
	b.WriteString(`</article>`)
	return b.String(), nil
}

// i18nSlug strips a folder's ordering prefix (e.g. "10-training" ->
// "training") to get a stable i18n key that survives reordering teasers by
// renaming their numeric prefix.
func i18nSlug(dirName string) string {
	if i := strings.IndexByte(dirName, '-'); i != -1 {
		if _, err := strconv.Atoi(dirName[:i]); err == nil {
			return dirName[i+1:]
		}
	}
	return dirName
}

func listImages(dirPath string) ([]string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func altFor(cfg teaserConfig, i int) string {
	if i < len(cfg.ImageAlt) && cfg.ImageAlt[i] != "" {
		return cfg.ImageAlt[i]
	}
	return cfg.Title + " screenshot " + strconv.Itoa(i+1)
}

// renderImageBlock renders nothing for a text-only teaser, a single <img>
// for one image, or cross-fade carousel markup (consumed by carousel.js)
// for two or more.
func renderImageBlock(slug string, images []string, cfg teaserConfig) string {
	if len(images) == 0 {
		return ""
	}

	srcFor := func(file string) string {
		return path.Join(imageURLPrefix, slug, file)
	}

	if len(images) == 1 {
		var b strings.Builder
		b.WriteString(`  <div class="bg-gray-100 border-b border-gray-200 p-3 pb-0">` + "\n")
		b.WriteString(`    <div class="relative aspect-[4/3] rounded-t-xl overflow-hidden bg-slate-50">` + "\n")
		fmt.Fprintf(&b, `      <img src="%s" alt="%s" class="absolute inset-0 w-full h-full object-contain">`+"\n",
			html.EscapeString(srcFor(images[0])), html.EscapeString(altFor(cfg, 0)))
		b.WriteString(`    </div>` + "\n")
		b.WriteString(`  </div>` + "\n")
		return b.String()
	}

	var b strings.Builder
	b.WriteString(`  <div class="bg-gray-100 border-b border-gray-200 p-3 pb-0">` + "\n")
	b.WriteString(`    <div class="relative aspect-[4/3] rounded-t-xl overflow-hidden bg-slate-50" data-carousel>` + "\n")
	for i, file := range images {
		activeClass := "opacity-0"
		if i == 0 {
			activeClass = "opacity-100"
		}
		fmt.Fprintf(&b, `      <img src="%s" alt="%s" data-slide="%d" class="carousel-slide absolute inset-0 w-full h-full object-contain transition-opacity duration-500 %s">`+"\n",
			html.EscapeString(srcFor(file)), html.EscapeString(altFor(cfg, i)), i, activeClass)
	}
	b.WriteString(`      <div class="absolute inset-x-0 bottom-0 flex justify-center gap-0.5 bg-gradient-to-t from-slate-900/10 to-transparent pt-6">` + "\n")
	for i := range images {
		dotClass := "bg-gray-900/25"
		if i == 0 {
			dotClass = "bg-blue-600"
		}
		fmt.Fprintf(&b, `        <button type="button" class="carousel-dot flex items-end justify-center w-11 h-11 pb-3" data-slide="%d" aria-label="Show image %d">`+"\n", i, i+1)
		fmt.Fprintf(&b, `          <span class="carousel-mark w-1.5 h-1.5 rounded-full ring-2 ring-white/80 %s"></span>`+"\n", dotClass)
		b.WriteString(`        </button>` + "\n")
	}
	b.WriteString(`      </div>` + "\n")
	b.WriteString(`    </div>` + "\n")
	b.WriteString(`  </div>` + "\n")
	return b.String()
}

// spliceMarkers replaces the content between startMarker and endMarker in
// doc with inner. Running it again on its own output with the same inner
// value is a no-op, so regeneration is idempotent and diff-friendly.
func spliceMarkers(doc, inner string) (string, error) {
	startIdx := strings.Index(doc, startMarker)
	if startIdx == -1 {
		return "", fmt.Errorf("marker %q not found", startMarker)
	}
	contentStart := startIdx + len(startMarker)
	endIdx := strings.Index(doc[contentStart:], endMarker)
	if endIdx == -1 {
		return "", fmt.Errorf("marker %q not found", endMarker)
	}
	endIdx += contentStart

	return doc[:contentStart] + "\n" + inner + "\n" + doc[endIdx:], nil
}

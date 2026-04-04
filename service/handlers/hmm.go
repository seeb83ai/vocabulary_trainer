package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

type HMMHandler struct {
	Store *db.Store
}

// ── Library endpoints ───────────────────────────────────────────────────

func (h *HMMHandler) GetActors(w http.ResponseWriter, r *http.Request) {
	actors, err := h.Store.GetHMMActors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch actors")
		return
	}
	if actors == nil {
		actors = []models.HMMActor{}
	}
	writeJSON(w, http.StatusOK, actors)
}

func (h *HMMHandler) UpdateActor(w http.ResponseWriter, r *http.Request) {
	initial := chi.URLParam(r, "initial")
	var body struct {
		ActorName string `json:"actor_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.Store.UpdateHMMActor(r.Context(), initial, strings.TrimSpace(body.ActorName)); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HMMHandler) GetLocations(w http.ResponseWriter, r *http.Request) {
	locs, err := h.Store.GetHMMLocations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch locations")
		return
	}
	if locs == nil {
		locs = []models.HMMLocation{}
	}
	writeJSON(w, http.StatusOK, locs)
}

func (h *HMMHandler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	finalKey := chi.URLParam(r, "final")
	var body struct {
		LocationName string `json:"location_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.Store.UpdateHMMLocation(r.Context(), finalKey, strings.TrimSpace(body.LocationName)); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HMMHandler) GetToneRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.Store.GetHMMToneRooms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch tone rooms")
		return
	}
	if rooms == nil {
		rooms = []models.HMMToneRoom{}
	}
	writeJSON(w, http.StatusOK, rooms)
}

func (h *HMMHandler) UpdateToneRoom(w http.ResponseWriter, r *http.Request) {
	tone, err := strconv.Atoi(chi.URLParam(r, "tone"))
	if err != nil || tone < 1 || tone > 5 {
		writeError(w, http.StatusBadRequest, "tone must be 1-5")
		return
	}
	var body struct {
		RoomName string `json:"room_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.Store.UpdateHMMToneRoom(r.Context(), tone, strings.TrimSpace(body.RoomName)); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HMMHandler) GetProps(w http.ResponseWriter, r *http.Request) {
	props, err := h.Store.GetHMMProps(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch props")
		return
	}
	if props == nil {
		props = []models.HMMProp{}
	}
	writeJSON(w, http.StatusOK, props)
}

func (h *HMMHandler) UpsertProp(w http.ResponseWriter, r *http.Request) {
	var body models.HMMProp
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Radical = strings.TrimSpace(body.Radical)
	if body.Radical == "" {
		writeError(w, http.StatusBadRequest, "radical is required")
		return
	}
	if err := h.Store.UpsertHMMProp(r.Context(), body.Radical, strings.TrimSpace(body.PropName)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HMMHandler) DeleteProp(w http.ResponseWriter, r *http.Request) {
	radical := chi.URLParam(r, "radical")
	if err := h.Store.DeleteHMMProp(r.Context(), radical); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── Scene endpoints ─────────────────────────────────────────────────────

func (h *HMMHandler) GetSceneContext(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid word id")
		return
	}
	word, err := h.Store.GetWordByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch word")
		return
	}
	if word == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	// Parse pinyin (parsePinyin already handles multi-syllable by taking the first)
	var initial, final string
	var tone int
	if word.Pinyin != nil && *word.Pinyin != "" {
		initial, final, tone = parsePinyin(*word.Pinyin)
	}

	runes := []rune(word.ZhText)
	isMultiChar := len(runes) > 1

	var radicals []string
	radicalDefs := map[string]string{}
	var decompositionStr string

	if isMultiChar {
		// For multi-character words each character becomes a prop.
		for _, ru := range runes {
			radicals = append(radicals, string(ru))
		}
		// Use English translations from the words table as placeholder hints.
		radicalDefs, _ = h.Store.GetEnTranslationsByZhTexts(r.Context(), radicals)
		if radicalDefs == nil {
			radicalDefs = map[string]string{}
		}
	} else {
		// Single character: decompose into components.
		decomps, _ := h.Store.GetHanziDecomposition(r.Context(), runes)
		if len(decomps) > 0 {
			radicals = collectRadicals(decomps[0])
			radicalDefs = collectRadicalDefs(decomps[0])
			decompositionStr = decomps[0].Decomposition
		}
	}

	ctx := r.Context()

	resp := models.HMMSceneContext{
		Initial:       initial,
		Final:         final,
		Tone:          tone,
		Decomposition: decompositionStr,
		Radicals:      radicals,
		RadicalDefs:   radicalDefs,
		MultiChar:     isMultiChar,
	}

	if initial != "" {
		resp.Actor, _ = h.Store.GetHMMActorByInitial(ctx, initial)
	}
	if final != "" {
		resp.Location, _ = h.Store.GetHMMLocationByFinal(ctx, final)
	}
	if tone >= 1 && tone <= 5 {
		resp.ToneRoom, _ = h.Store.GetHMMToneRoom(ctx, tone)
	}
	if len(radicals) > 0 {
		resp.Props, _ = h.Store.GetHMMPropsByRadicals(ctx, radicals)
	}
	if resp.Props == nil {
		resp.Props = []models.HMMProp{}
	}
	if resp.Radicals == nil {
		resp.Radicals = []string{}
	}

	scene, _ := h.Store.GetHMMScene(ctx, id)
	resp.Scene = scene

	writeJSON(w, http.StatusOK, resp)
}

func (h *HMMHandler) SaveScene(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid word id")
		return
	}
	word, err := h.Store.GetWordByID(r.Context(), id)
	if err != nil || word == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	var req models.HMMSaveSceneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Parse pinyin to determine initial/final/tone for library updates
	var initial, final string
	var tone int
	if word.Pinyin != nil && *word.Pinyin != "" {
		initial, final, tone = parsePinyin(*word.Pinyin)
	}

	if err := h.Store.SaveHMMSceneWithLibrary(r.Context(), id, initial, final, tone, req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save scene")
		return
	}

	if req.Decomposition != "" {
		_ = h.Store.UpsertHanziDecomposition(r.Context(), word.ZhText, req.Decomposition)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HMMHandler) DeleteScene(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid word id")
		return
	}
	if err := h.Store.DeleteHMMScene(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete scene")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── Pinyin parsing ──────────────────────────────────────────────────────

// toneMap maps accented vowels to their base character and tone number.
var toneMap = map[rune][2]any{
	'ā': {'a', 1}, 'á': {'a', 2}, 'ǎ': {'a', 3}, 'à': {'a', 4},
	'ō': {'o', 1}, 'ó': {'o', 2}, 'ǒ': {'o', 3}, 'ò': {'o', 4},
	'ē': {'e', 1}, 'é': {'e', 2}, 'ě': {'e', 3}, 'è': {'e', 4},
	'ī': {'i', 1}, 'í': {'i', 2}, 'ǐ': {'i', 3}, 'ì': {'i', 4},
	'ū': {'u', 1}, 'ú': {'u', 2}, 'ǔ': {'u', 3}, 'ù': {'u', 4},
	'ǖ': {'v', 1}, 'ǘ': {'v', 2}, 'ǚ': {'v', 3}, 'ǜ': {'v', 4},
}

// femaleInitials are consonants that combine with "i" to form a female-category initial.
var femaleConsonants = map[string]bool{
	"b": true, "p": true, "m": true, "d": true,
	"t": true, "n": true, "l": true,
}

// finalSimplify maps compound i/u/ü-prefixed finals to one of 13 base finals.
var finalSimplify = map[string]string{
	// i-prefixed
	"i": "null", "ia": "a", "ie": "e", "iao": "ao", "iu": "ou",
	"ian": "an", "in": "en", "iang": "ang", "ing": "eng", "iong": "ong",
	// u-prefixed
	"u": "null", "ua": "a", "uo": "o", "uai": "ai", "ui": "ei",
	"uan": "an", "un": "en", "uang": "ang", "ueng": "eng",
	// ü-prefixed (represented as v)
	"v": "null", "ve": "e", "van": "an", "vn": "en",
}

// parsePinyin splits a pinyin syllable into HMM initial, final, and tone.
// The initial maps to an actor, the final to a location, and the tone to a room.
func parsePinyin(syllable string) (initial, final string, tone int) {
	syllable = strings.TrimSpace(strings.ToLower(syllable))
	if syllable == "" {
		return "null", "null", 5
	}

	// If multi-syllable, take only the first syllable
	if idx := strings.IndexByte(syllable, ' '); idx > 0 {
		syllable = syllable[:idx]
	}

	// Strip tone marks and record tone
	tone = 5 // default: neutral
	var stripped []rune
	for _, r := range syllable {
		if m, ok := toneMap[r]; ok {
			stripped = append(stripped, m[0].(rune))
			tone = m[1].(int)
		} else {
			stripped = append(stripped, r)
		}
	}
	// Replace ü with v for consistent handling
	s := strings.ReplaceAll(string(stripped), "ü", "v")

	// Also handle numeric tone suffix (e.g. "da4")
	if tone == 5 && len(s) > 0 {
		last := s[len(s)-1]
		if last >= '1' && last <= '5' {
			tone = int(last - '0')
			s = s[:len(s)-1]
		}
	}

	// Identify consonant initial
	consonant := ""
	remainder := s
	if strings.HasPrefix(s, "zh") || strings.HasPrefix(s, "ch") || strings.HasPrefix(s, "sh") {
		consonant = s[:2]
		remainder = s[2:]
	} else if len(s) > 0 {
		first := s[0]
		if first >= 'a' && first <= 'z' && !isVowel(rune(first)) {
			consonant = s[:1]
			remainder = s[1:]
		}
	}

	if consonant == "" {
		// No consonant — null initial
		initial = "null"
		final = simplifyFinal(remainder)
		if final == "" {
			final = "null"
		}
		return
	}

	// Check for female initial (consonant + i)
	if femaleConsonants[consonant] && strings.HasPrefix(remainder, "i") {
		initial = consonant + "i"
		final = simplifyFinal(remainder)
	} else {
		initial = consonant
		final = simplifyFinal(remainder)
	}
	if final == "" {
		final = "null"
	}
	return
}

func simplifyFinal(f string) string {
	if f == "" {
		return "null"
	}
	if simplified, ok := finalSimplify[f]; ok {
		return simplified
	}
	// Already a base final (a, o, e, ai, ei, ao, ou, an, ang, en, eng, ong, er)
	return f
}

func isVowel(r rune) bool {
	return r == 'a' || r == 'e' || r == 'i' || r == 'o' || r == 'u' || r == 'v'
}

// ── Helpers ─────────────────────────────────────────────────────────────

// collectRadicals extracts unique component characters from a decomposition tree.
func collectRadicals(d models.HanziDecomposition) []string {
	seen := map[string]bool{}
	var result []string
	var walk func(d models.HanziDecomposition)
	walk = func(d models.HanziDecomposition) {
		for _, c := range d.Components {
			ch := c.Character
			if ch != "" && !seen[ch] && utf8.RuneCountInString(ch) == 1 {
				seen[ch] = true
				result = append(result, ch)
			}
			walk(c)
		}
	}
	walk(d)
	return result
}

// collectRadicalDefs returns a map of radical character → definition from a decomposition tree.
func collectRadicalDefs(d models.HanziDecomposition) map[string]string {
	defs := map[string]string{}
	var walk func(d models.HanziDecomposition)
	walk = func(d models.HanziDecomposition) {
		for _, c := range d.Components {
			if c.Character != "" && utf8.RuneCountInString(c.Character) == 1 && c.Definition != "" {
				if _, exists := defs[c.Character]; !exists {
					defs[c.Character] = c.Definition
				}
			}
			walk(c)
		}
	}
	walk(d)
	return defs
}

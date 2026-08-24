package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"vocabulary_trainer/db"
	"vocabulary_trainer/sm2"
)

// DemoHandler serves the unauthenticated try-before-signup quiz on the
// landing page. The card set is fixed and stateless: no user data is read
// or written, answers are checked with the same sm2.CheckAnswer logic the
// real quiz uses. Each answer is recorded to the audit log (user_id 0, like
// other pre-account events) so demo engagement is visible without touching
// the real quiz stats or funnel tables.
type DemoHandler struct {
	Store *db.Store
}

type demoCard struct {
	ID           int      `json:"id"`
	Zh           string   `json:"zh"`
	Pinyin       string   `json:"pinyin"`
	Translations []string `json:"translations,omitempty"`
}

// demoCardSet is served in fixed order so the demo is deterministic.
var demoCardSet = []demoCard{
	{ID: 0, Zh: "你好", Pinyin: "nǐ hǎo", Translations: []string{"hello", "hi"}},
	{ID: 1, Zh: "谢谢", Pinyin: "xiè xie", Translations: []string{"thank you", "thanks"}},
	{ID: 2, Zh: "猫", Pinyin: "māo", Translations: []string{"cat"}},
	{ID: 3, Zh: "水", Pinyin: "shuǐ", Translations: []string{"water"}},
	{ID: 4, Zh: "中国", Pinyin: "zhōng guó", Translations: []string{"China"}},
}

// Cards handles GET /api/demo/cards — returns the demo card list without
// translations, so the answer cannot be read from the payload.
func (h *DemoHandler) Cards(w http.ResponseWriter, r *http.Request) {
	cards := make([]demoCard, len(demoCardSet))
	for i, c := range demoCardSet {
		cards[i] = demoCard{ID: c.ID, Zh: c.Zh, Pinyin: c.Pinyin}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": cards})
}

// Answer handles POST /api/demo/answer — checks a demo answer statelessly
// and reveals the accepted translations for the result screen.
func (h *DemoHandler) Answer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CardID int    `json:"card_id"`
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.CardID < 0 || req.CardID >= len(demoCardSet) {
		writeError(w, http.StatusBadRequest, "invalid card_id")
		return
	}
	card := demoCardSet[req.CardID]
	correct := sm2.CheckAnswer(req.Answer, card.Translations)

	detail := fmt.Sprintf("card_id=%d correct=%v", card.ID, correct)
	_ = h.Store.RecordAuditLog(r.Context(), 0, db.AuditDemoAnswer, ClientIP(r), detail)

	writeJSON(w, http.StatusOK, map[string]any{
		"correct":      correct,
		"translations": card.Translations,
	})
}

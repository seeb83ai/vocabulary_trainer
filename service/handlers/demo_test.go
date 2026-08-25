package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"vocabulary_trainer/db"
)

func TestDemoCards_ReturnsFiveCardsWithoutTranslations(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/demo/cards", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body struct {
		Cards []struct {
			ID           int      `json:"id"`
			Zh           string   `json:"zh"`
			Pinyin       string   `json:"pinyin"`
			Translations []string `json:"translations"`
		} `json:"cards"`
	}
	decodeJSON(t, rec, &body)
	if len(body.Cards) != 5 {
		t.Fatalf("want 5 demo cards, got %d", len(body.Cards))
	}
	for i, c := range body.Cards {
		if c.Zh == "" || c.Pinyin == "" {
			t.Errorf("card %d: missing zh or pinyin: %+v", i, c)
		}
		if len(c.Translations) != 0 {
			t.Errorf("card %d: translations must not be exposed in the card list", i)
		}
	}
}

func TestDemoAnswer_CorrectWrongAndNormalized(t *testing.T) {
	r := newRouter(openTestDB(t))

	cases := []struct {
		answer  string
		correct bool
	}{
		{"hello", true},
		{"  HELLO ", true}, // normalization applies
		{"zzz", false},
	}
	for _, c := range cases {
		rec := do(t, r, "POST", "/api/demo/answer", map[string]any{"card_id": 0, "answer": c.answer})
		if rec.Code != http.StatusOK {
			t.Fatalf("answer %q: want 200, got %d", c.answer, rec.Code)
		}
		var body struct {
			Correct      bool     `json:"correct"`
			Translations []string `json:"translations"`
		}
		decodeJSON(t, rec, &body)
		if body.Correct != c.correct {
			t.Errorf("answer %q: want correct=%v, got %v", c.answer, c.correct, body.Correct)
		}
		if len(body.Translations) == 0 {
			t.Errorf("answer %q: result must reveal the accepted translations", c.answer)
		}
	}
}

func TestDemoAnswer_InvalidRequests(t *testing.T) {
	r := newRouter(openTestDB(t))

	rec := do(t, r, "POST", "/api/demo/answer", map[string]any{"card_id": 99, "answer": "x"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("out-of-range card_id: want 400, got %d", rec.Code)
	}
	rec = do(t, r, "POST", "/api/demo/answer", map[string]any{"card_id": -1, "answer": "x"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("negative card_id: want 400, got %d", rec.Code)
	}
}

func TestDemoAnswer_RecordsAuditLogEntry(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/demo/answer", map[string]any{"card_id": 0, "answer": "hello"})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	// httptest.NewRequest defaults RemoteAddr to 192.0.2.1:1234.
	entries, err := s.GetAuditLogByIP(context.Background(), "192.0.2.1", 10)
	if err != nil {
		t.Fatalf("GetAuditLogByIP: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 audit entry, got %d", len(entries))
	}
	if entries[0].Action != db.AuditDemoAnswer {
		t.Errorf("action: want %q, got %q", db.AuditDemoAnswer, entries[0].Action)
	}
	// user_id stays 0 — the demo has no account, matching the existing
	// convention for pre-account audit events (e.g. failed logins).
	if entries[0].UserID != 0 {
		t.Errorf("user_id: want 0, got %d", entries[0].UserID)
	}
}

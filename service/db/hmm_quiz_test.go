package db

import (
	"context"
	"database/sql"
	"testing"
	"vocabulary_trainer/models"
)

// clearAllHMMNames blanks every library entry name so no entries qualify as
// named. Migration v13 seeds tone rooms (with names) and props; this resets
// them all so tests start from a predictable blank slate.
func clearAllHMMNames(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`UPDATE hmm_actors SET actor_name = ''`,
		`UPDATE hmm_locations SET location_name = ''`,
		`UPDATE hmm_tone_rooms SET room_name = ''`,
		`UPDATE hmm_props SET prop_name = ''`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("clearAllHMMNames: %v", err)
		}
	}
}

// seedHMMLibrary sets up exactly 4 named entries: one of each entity type.
func seedHMMLibrary(t *testing.T, db *sql.DB) {
	t.Helper()
	clearAllHMMNames(t, db)
	stmts := []string{
		`UPDATE hmm_actors SET actor_name = 'Bruce Lee' WHERE initial = 'b'`,
		`UPDATE hmm_locations SET location_name = 'Grand Canyon' WHERE final_key = 'an'`,
		`UPDATE hmm_tone_rooms SET room_name = 'Entrance' WHERE tone = 1`,
		`INSERT OR IGNORE INTO hmm_props (radical, prop_name) VALUES ('一', 'razor blade')`,
		`UPDATE hmm_props SET prop_name = 'razor blade' WHERE radical = '一'`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seedHMMLibrary: %v", err)
		}
	}
}

func TestEnsureHMMProgress(t *testing.T) {
	store := openTestDB(t)
	seedHMMLibrary(t, store.db)

	if err := store.EnsureHMMProgress(context.Background()); err != nil {
		t.Fatalf("EnsureHMMProgress: %v", err)
	}

	// Named entries should have progress rows
	for _, key := range []struct{ typ, key string }{
		{"actor", "b"},
		{"location", "an"},
		{"tone_room", "1"},
		{"prop", "一"},
	} {
		prog, err := store.GetHMMProgress(context.Background(), key.typ, key.key)
		if err != nil {
			t.Fatalf("GetHMMProgress %s/%s: %v", key.typ, key.key, err)
		}
		if prog == nil {
			t.Errorf("expected progress row for %s/%s, got nil", key.typ, key.key)
		}
	}

	// Unnamed entries (e.g. initial 'p' with empty actor_name) should have no row
	prog, err := store.GetHMMProgress(context.Background(), "actor", "p")
	if err != nil {
		t.Fatalf("GetHMMProgress actor/p: %v", err)
	}
	if prog != nil {
		t.Error("expected nil progress for unnamed actor 'p'")
	}

	// Calling EnsureHMMProgress again is safe (INSERT OR IGNORE)
	if err := store.EnsureHMMProgress(context.Background()); err != nil {
		t.Fatalf("EnsureHMMProgress second call: %v", err)
	}
}

func TestGetNextHMMCard_NoRows(t *testing.T) {
	store := openTestDB(t)
	clearAllHMMNames(t, store.db)

	card, prog, err := store.GetNextHMMCard(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextHMMCard: %v", err)
	}
	if card != nil || prog != nil {
		t.Error("expected nil card/prog when no named entries exist")
	}
}

func TestGetNextHMMCard_ReturnsCard(t *testing.T) {
	store := openTestDB(t)
	seedHMMLibrary(t, store.db)

	if err := store.EnsureHMMProgress(context.Background()); err != nil {
		t.Fatalf("EnsureHMMProgress: %v", err)
	}

	card, prog, err := store.GetNextHMMCard(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextHMMCard: %v", err)
	}
	if card == nil {
		t.Fatal("expected a card, got nil")
	}
	if prog == nil {
		t.Fatal("expected progress, got nil")
	}
	if card.EntityType == "" || card.EntityKey == "" {
		t.Errorf("card has empty entity_type or entity_key: %+v", card)
	}
	if card.Prompt == "" {
		t.Error("card.Prompt is empty")
	}
}

func TestGetNextHMMCard_TypeFilter(t *testing.T) {
	store := openTestDB(t)
	seedHMMLibrary(t, store.db)

	if err := store.EnsureHMMProgress(context.Background()); err != nil {
		t.Fatalf("EnsureHMMProgress: %v", err)
	}

	// Request only actors
	card, _, err := store.GetNextHMMCard(context.Background(), []string{models.HMMEntityActor})
	if err != nil {
		t.Fatalf("GetNextHMMCard actor filter: %v", err)
	}
	if card == nil {
		t.Fatal("expected actor card, got nil")
	}
	if card.EntityType != models.HMMEntityActor {
		t.Errorf("got entity_type %q, want %q", card.EntityType, models.HMMEntityActor)
	}

	// Request only props — should get the prop
	card, _, err = store.GetNextHMMCard(context.Background(), []string{models.HMMEntityProp})
	if err != nil {
		t.Fatalf("GetNextHMMCard prop filter: %v", err)
	}
	if card == nil {
		t.Fatal("expected prop card, got nil")
	}
	if card.EntityType != models.HMMEntityProp {
		t.Errorf("got entity_type %q, want %q", card.EntityType, models.HMMEntityProp)
	}
}

func TestGetHMMProgress_RoundTrip(t *testing.T) {
	store := openTestDB(t)
	seedHMMLibrary(t, store.db)

	if err := store.EnsureHMMProgress(context.Background()); err != nil {
		t.Fatalf("EnsureHMMProgress: %v", err)
	}

	prog, err := store.GetHMMProgress(context.Background(), "actor", "b")
	if err != nil {
		t.Fatalf("GetHMMProgress: %v", err)
	}
	if prog == nil {
		t.Fatal("expected progress row")
	}

	// Mutate and save
	prog.Repetitions = 3
	prog.Easiness = 2.8
	prog.TotalCorrect = 3
	prog.TotalAttempts = 4
	prog.Learning = false

	if err := store.UpdateHMMProgress(context.Background(), *prog); err != nil {
		t.Fatalf("UpdateHMMProgress: %v", err)
	}

	// Read back
	got, err := store.GetHMMProgress(context.Background(), "actor", "b")
	if err != nil {
		t.Fatalf("GetHMMProgress after update: %v", err)
	}
	if got.Repetitions != 3 {
		t.Errorf("Repetitions = %d, want 3", got.Repetitions)
	}
	if got.Easiness != 2.8 {
		t.Errorf("Easiness = %f, want 2.8", got.Easiness)
	}
	if got.Learning {
		t.Error("Learning should be false")
	}
}

func TestAcknowledgeHMMEntry(t *testing.T) {
	store := openTestDB(t)
	seedHMMLibrary(t, store.db)

	if err := store.EnsureHMMProgress(context.Background()); err != nil {
		t.Fatalf("EnsureHMMProgress: %v", err)
	}

	// Before acknowledge: first_seen_date is NULL
	prog, _ := store.GetHMMProgress(context.Background(), "actor", "b")
	if prog.FirstSeenDate != "" {
		t.Errorf("expected empty first_seen_date, got %q", prog.FirstSeenDate)
	}

	if err := store.AcknowledgeHMMEntry(context.Background(), "actor", "b"); err != nil {
		t.Fatalf("AcknowledgeHMMEntry: %v", err)
	}

	// After acknowledge: first_seen_date is set
	prog, _ = store.GetHMMProgress(context.Background(), "actor", "b")
	if prog.FirstSeenDate == "" {
		t.Error("expected first_seen_date to be set after acknowledge")
	}

	// Calling again is a no-op (stays the same)
	original := prog.FirstSeenDate
	if err := store.AcknowledgeHMMEntry(context.Background(), "actor", "b"); err != nil {
		t.Fatalf("AcknowledgeHMMEntry second call: %v", err)
	}
	prog, _ = store.GetHMMProgress(context.Background(), "actor", "b")
	if prog.FirstSeenDate != original {
		t.Errorf("first_seen_date changed on second acknowledge: %q -> %q", original, prog.FirstSeenDate)
	}
}

func TestGetHMMStats(t *testing.T) {
	store := openTestDB(t)
	seedHMMLibrary(t, store.db)

	if err := store.EnsureHMMProgress(context.Background()); err != nil {
		t.Fatalf("EnsureHMMProgress: %v", err)
	}

	stats, err := store.GetHMMStats(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetHMMStats: %v", err)
	}

	// We seeded 4 named entries
	if stats.Total != 4 {
		t.Errorf("Total = %d, want 4", stats.Total)
	}
	// All entries have due_date = CURRENT_TIMESTAMP, so all 4 are due
	if stats.DueToday != 4 {
		t.Errorf("DueToday = %d, want 4", stats.DueToday)
	}

	// Type filter: only actors
	stats, err = store.GetHMMStats(context.Background(), []string{models.HMMEntityActor})
	if err != nil {
		t.Fatalf("GetHMMStats actor filter: %v", err)
	}
	if stats.Total != 1 {
		t.Errorf("actor Total = %d, want 1", stats.Total)
	}
}

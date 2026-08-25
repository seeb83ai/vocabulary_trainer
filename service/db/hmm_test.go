package db

import (
	"context"
	"testing"
)

func TestImportTemplateWords_CopiesWordsForUser(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "苹果", "píngguǒ", []string{"apple"}, nil)

	userID := insertTestUser(t, s, "test@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatalf("count user words: %v", err)
	}
	if count == 0 {
		t.Error("expected user to have words after ImportTemplateWords")
	}
}

func TestImportTemplateWords_CreatesSM2Progress(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "猫", "māo", []string{"cat"}, nil)

	userID := insertTestUser(t, s, "test2@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sm2_progress sp
		JOIN words w ON w.id = sp.word_id
		WHERE w.user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatalf("count sm2_progress: %v", err)
	}
	if count == 0 {
		t.Error("expected sm2_progress rows for imported words")
	}
}

func TestImportTemplateWords_CopiesTranslations(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "书", "shū", []string{"book"}, nil)

	userID := insertTestUser(t, s, "test3@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	// The zh word imported for the user should have a translation linked to an en word.
	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM translations t
		JOIN words zh ON zh.id = t.zh_word_id AND zh.user_id = ?
		JOIN words en ON en.id = t.translation_word_id
	`, userID).Scan(&count); err != nil {
		t.Fatalf("count translations: %v", err)
	}
	if count == 0 {
		t.Error("expected translations to be copied for imported words")
	}
}

func TestImportTemplateWords_Idempotent(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "水", "shuǐ", []string{"water"}, nil)

	userID := insertTestUser(t, s, "test4@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("first ImportTemplateWords: %v", err)
	}
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("second ImportTemplateWords: %v", err)
	}

	// Should still have only one zh word per template.
	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM words WHERE user_id = ? AND language = 'zh'`, userID).Scan(&count); err != nil {
		t.Fatalf("count zh words: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 zh word after idempotent import, got %d", count)
	}
}

func TestImportTemplateWords_TemplatesUnchanged(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "火", "huǒ", []string{"fire"}, nil)

	userID := insertTestUser(t, s, "test5@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	// Template words (user_id=1, admin) must still exist after import.
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = 1 AND language = 'zh'`).Scan(&count); err != nil {
		t.Fatalf("count template zh words: %v", err)
	}
	if count == 0 {
		t.Error("template words should remain after ImportTemplateWords")
	}
}

func TestImportTemplateWords_NoSM2ForTemplates(t *testing.T) {
	s := openTestDB(t)
	tmplID := seedTemplateWord(t, s, "地", "dì", []string{"earth", "ground"}, nil)

	// Count sm2_progress for the template word before import (CreateWord creates one).
	var before int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sm2_progress WHERE word_id = ?`, tmplID).Scan(&before); err != nil {
		t.Fatalf("count sm2_progress before import: %v", err)
	}

	userID := insertTestUser(t, s, "test6@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	// ImportTemplateWords must not modify the template word's sm2_progress.
	var after int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sm2_progress WHERE word_id = ?`, tmplID).Scan(&after); err != nil {
		t.Fatalf("count sm2_progress after import: %v", err)
	}
	if after != before {
		t.Errorf("import changed template sm2_progress count: before=%d, after=%d", before, after)
	}
}

func insertTestUser(t *testing.T, s *Store, email string) int64 {
	t.Helper()
	res, err := s.db.Exec(`INSERT INTO users (email, password_hash) VALUES (?, 'x')`, email)
	if err != nil {
		t.Fatalf("insert test user %q: %v", email, err)
	}
	id, _ := res.LastInsertId()
	return id
}

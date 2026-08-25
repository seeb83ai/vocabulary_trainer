package db

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestMigration_v20_UsersTableExists(t *testing.T) {
	s := openTestDB(t)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Error("users table should exist after migration v20")
	}
}

func TestMigration_v20_BothUsersSeeded(t *testing.T) {
	s := openTestDB(t)

	var adminHash, meHash string
	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE email = 'admin@example.de'`).Scan(&adminHash); err != nil {
		t.Fatalf("query admin user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash), []byte("I am the admin")); err != nil {
		t.Errorf("admin password hash does not match 'I am the admin': %v", err)
	}

	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE email = 'me@example.de'`).Scan(&meHash); err != nil {
		t.Fatalf("query initial user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(meHash), []byte("I learn zh")); err != nil {
		t.Errorf("me password hash does not match 'I learn zh': %v", err)
	}
}

func TestMigration_v20_AdminIsUserID1(t *testing.T) {
	s := openTestDB(t)
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM users WHERE email = 'admin@example.de'`).Scan(&id); err != nil {
		t.Fatalf("query admin id: %v", err)
	}
	if id != 1 {
		t.Errorf("expected admin user id=1, got %d", id)
	}
}

func TestMigration_v20_IdempotentOnFreshDB(t *testing.T) {
	s := openTestDB(t)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 2 {
		t.Errorf("expected exactly 2 users after migration, got %d", count)
	}
}

func TestMigration_v21_WordsHaveUserIDColumn(t *testing.T) {
	s := openTestDB(t)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('words') WHERE name = 'user_id'`).Scan(&count); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	if count != 1 {
		t.Error("words table should have a user_id column after migration v21")
	}
}

func TestMigration_v21_CreateWordSetsUserID(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "测试", "cè shì", []string{"test"})

	var userID int64
	if err := s.db.QueryRow(`SELECT user_id FROM words WHERE id = ?`, id).Scan(&userID); err != nil {
		t.Fatalf("query word user_id: %v", err)
	}
	if userID != 2 {
		t.Errorf("expected user_id=2 for word created via CreateWord, got %d", userID)
	}
}

func TestMigration_v21_TemplateWordsAreSubsetOfAllWords(t *testing.T) {
	s := openTestDB(t)
	// Insert a template word (admin user, id=1) and a regular word (me user, id=2).
	seedTemplateWord(t, s, "学习", "xuéxí", []string{"study"}, nil)
	seedWord(t, s, "工作", "gōngzuò", []string{"work"})

	var templateCount, totalCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = 1`).Scan(&templateCount); err != nil {
		t.Fatalf("count template words: %v", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words`).Scan(&totalCount); err != nil {
		t.Fatalf("count all words: %v", err)
	}
	if templateCount > totalCount {
		t.Errorf("template words (%d) must not exceed total words (%d)", templateCount, totalCount)
	}
}

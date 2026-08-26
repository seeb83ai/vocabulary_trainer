package db

import (
	"context"
	"testing"
)

func TestGetWordImageURL_DefaultNil(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "猫", "māo", []string{"cat"})

	url, err := s.GetWordImageURL(ctx, int64(2), id)
	if err != nil {
		t.Fatalf("GetWordImageURL: %v", err)
	}
	if url != nil {
		t.Errorf("want nil image url before any fetch, got %q", *url)
	}
}

func TestSetWordImageURL_ThenGet(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "猫", "māo", []string{"cat"})

	if err := s.SetWordImageURL(ctx, int64(2), id, "https://images.example/cat.jpg"); err != nil {
		t.Fatalf("SetWordImageURL: %v", err)
	}
	url, err := s.GetWordImageURL(ctx, int64(2), id)
	if err != nil {
		t.Fatalf("GetWordImageURL after set: %v", err)
	}
	if url == nil || *url != "https://images.example/cat.jpg" {
		t.Errorf("want cached url, got %v", url)
	}
}

func TestGetWordImageURL_WrongOwnerReturnsNil(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "猫", "māo", []string{"cat"})
	if err := s.SetWordImageURL(ctx, int64(2), id, "https://images.example/cat.jpg"); err != nil {
		t.Fatalf("SetWordImageURL: %v", err)
	}

	url, err := s.GetWordImageURL(ctx, int64(999), id)
	if err != nil {
		t.Fatalf("GetWordImageURL: %v", err)
	}
	if url != nil {
		t.Errorf("want nil for non-owner, got %q", *url)
	}
}

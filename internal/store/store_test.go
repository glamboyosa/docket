package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/glamboyosa/docket/internal/domain"
)

func TestCreateAndCompleteDocument(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "docket.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	doc := &domain.Document{SHA256: "abc", OriginalName: "record.pdf", SourcePath: "/source/record.pdf", LibraryPath: "/library/inbox/record.pdf", Status: domain.StatusQueued, Provider: "openai", Model: "test-model"}
	if err := s.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}
	classification := domain.Classification{Category: "tax", CategoryConfidence: .91, CategoryProbabilities: map[string]float64{"tax": .91}, Sensitivity: 2.1, Urgency: 1.2, NeedsAction: true, NeedsActionProbability: .88}
	if err := s.Complete(ctx, doc.ID, "/library/tax/record.pdf", classification); err != nil {
		t.Fatal(err)
	}
	got, err := s.ByID(ctx, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFiled || got.Category != "tax" || !got.NeedsAction {
		t.Fatalf("unexpected document: %+v", got)
	}
}

func TestActionReviewPersistsWithoutChangingCategory(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "docket.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	doc := &domain.Document{SHA256: "review", OriginalName: "notice.txt", Status: domain.StatusQueued}
	if err := s.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}
	c := domain.Classification{Category: "housing", CategoryConfidence: .93, ActionAssessment: "unclear", ActionConfidence: .8, ActionReview: true}
	if err := s.Complete(ctx, doc.ID, "/library/housing/notice.txt", c); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActionResolution(ctx, doc.ID, "not_applicable"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ByID(ctx, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "housing" || got.Review || !got.ActionReview || got.ActionResolution != "not_applicable" || got.ActionAssessment != "unclear" || got.ActionConfidence != .8 {
		t.Fatalf("unexpected document after action review: %+v", got)
	}
	if err := s.SetActionResolution(ctx, doc.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, err = s.ByID(ctx, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActionResolution != "" || !got.ActionReview {
		t.Fatalf("action review was not reopened: %+v", got)
	}
}

func TestExistingDatabaseGainsActionColumns(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "docket.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE documents (
		id INTEGER PRIMARY KEY, sha256 TEXT NOT NULL UNIQUE, original_name TEXT NOT NULL,
		source_path TEXT NOT NULL, library_path TEXT NOT NULL, status TEXT NOT NULL,
		category TEXT NOT NULL DEFAULT '', category_confidence REAL NOT NULL DEFAULT 0,
		category_probabilities TEXT NOT NULL DEFAULT '{}', sensitivity REAL NOT NULL DEFAULT 0,
		urgency REAL NOT NULL DEFAULT 0, needs_action INTEGER NOT NULL DEFAULT 0,
		needs_action_probability REAL NOT NULL DEFAULT 0, review INTEGER NOT NULL DEFAULT 0,
		provider TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	doc := &domain.Document{SHA256: "legacy", OriginalName: "old.txt", Status: domain.StatusQueued}
	if err := s.Create(context.Background(), doc); err != nil {
		t.Fatal(err)
	}
	got, err := s.ByID(context.Background(), doc.ID)
	if err != nil || got == nil {
		t.Fatalf("read legacy database: %v", err)
	}
}

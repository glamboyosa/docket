package store

import (
	"context"
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

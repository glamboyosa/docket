package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/glamboyosa/docket/internal/domain"
	"github.com/glamboyosa/docket/internal/files"
	"github.com/glamboyosa/docket/internal/store"
)

type fakeExtractor struct{ text string }

func (f fakeExtractor) Extract(context.Context, string) (string, error) { return f.text, nil }

type fakeClassifier struct{ result domain.Classification }

func (f fakeClassifier) Classify(context.Context, string) (domain.Classification, error) {
	return f.result, nil
}

func TestProcessorCopiesClassifiesAndFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "docket.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	source := filepath.Join(t.TempDir(), "lease.txt")
	if err := os.WriteFile(source, []byte("lease"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &Processor{Store: s, Library: files.Library{Root: filepath.Join(root, "library")}, Extractor: fakeExtractor{"lease"}, Classifier: fakeClassifier{domain.Classification{Category: "housing", CategoryConfidence: .93, CategoryProbabilities: map[string]float64{"housing": .93}}}, Provider: "openai", Model: "test-model"}
	doc, err := p.Add(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != domain.StatusFiled || doc.Category != "housing" {
		t.Fatalf("unexpected document: %+v", doc)
	}
	if filepath.Dir(doc.LibraryPath) != filepath.Join(root, "library", "housing") {
		t.Fatalf("unexpected library path %q", doc.LibraryPath)
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "lease" {
		t.Fatalf("source was changed: %q, %v", data, err)
	}
}

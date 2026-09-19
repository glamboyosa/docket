package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/glamboyosa/docket/internal/config"
	"github.com/glamboyosa/docket/internal/domain"
	"github.com/glamboyosa/docket/internal/provider"
)

func TestLibraryViewShowsClassification(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.width, m.height = 100, 28
	updated, _ := m.Update(loadedMsg{docs: []domain.Document{{
		OriginalName: "rental-agreement.pdf", Category: "housing", CategoryConfidence: .94,
		Sensitivity: .8, Urgency: 1.6, NeedsAction: true, NeedsActionProbability: .82,
		Status: domain.StatusFiled, Provider: "openai", Model: "test-model", LibraryPath: "/library/housing/rental-agreement.pdf",
	}}})
	view := updated.(Model).View()
	for _, value := range []string{
		"rental-agreement.pdf", "HOUSING", "94%", "openai",
		"Personal · keep private", "Time-sensitive · act soon", "Likely required · 82%", "Act soon; check exact deadline",
		"housing · soon", "↑/↓ select", "a add", "s models",
	} {
		if !strings.Contains(view, value) {
			t.Fatalf("view does not contain %q", value)
		}
	}
	if strings.Contains(view, "/ 3") {
		t.Fatal("view exposes an unexplained raw score")
	}
}

func TestGuidanceEscalatesAmbiguousActionAndCategory(t *testing.T) {
	t.Parallel()
	doc := domain.Document{
		Category: "other", CategoryConfidence: .42, Review: true,
		Sensitivity: 2.7, Urgency: 1.8, NeedsActionProbability: .52,
	}
	view := strings.Join(guidanceRows(doc, 60), "\n")
	for _, value := range []string{"Check for a request or deadline", "Highly sensitive · secure carefully", "Unclear · 52%", "Required · verify category"} {
		if !strings.Contains(view, value) {
			t.Fatalf("guidance does not contain %q: %s", value, view)
		}
	}
}

func TestScoreGuidanceUsesNearestRubricLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		score       float64
		sensitivity string
		urgency     string
	}{
		{name: "level zero", score: .49, sensitivity: "Routine", urgency: "No deadline detected"},
		{name: "level one", score: .5, sensitivity: "Personal · keep private", urgency: "Can wait · plan follow-up"},
		{name: "level two", score: 1.5, sensitivity: "Confidential · limit sharing", urgency: "Time-sensitive · act soon"},
		{name: "level three", score: 2.5, sensitivity: "Highly sensitive · secure carefully", urgency: "Urgent · act now"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sensitivityGuidance(test.score); got != test.sensitivity {
				t.Fatalf("sensitivityGuidance(%v) = %q", test.score, got)
			}
			if got := urgencyGuidance(test.score); got != test.urgency {
				t.Fatalf("urgencyGuidance(%v) = %q", test.score, got)
			}
		})
	}
}

func TestEnterInspectsSelectedDocumentAtNarrowWidth(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.width, m.height = 72, 28
	m.docs = []domain.Document{{
		OriginalName: "notice.pdf", Category: "legal", CategoryConfidence: .9,
		Status: domain.StatusFiled, Sensitivity: 2, Urgency: 2.8, NeedsAction: true, NeedsActionProbability: .94,
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != inspectDocument {
		t.Fatalf("mode = %v", m.mode)
	}
	view := m.View()
	for _, value := range []string{"Document details", "Urgent · act now", "Act now; check exact deadline"} {
		if !strings.Contains(view, value) {
			t.Fatalf("detail view does not contain %q", value)
		}
	}
}

func TestAddFlowCleansDraggedPath(t *testing.T) {
	t.Parallel()
	var added string
	m := New(Dependencies{
		Config: config.Config{Provider: "openai", Model: "test-model"},
		Add: func(_ context.Context, path string) (int, error) {
			added = path
			return 1, nil
		},
	})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	for _, value := range []rune("[/tmp/Test\\ File.pdf]") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}})
		m = updated.(Model)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !m.busy || cmd == nil {
		t.Fatal("expected import command to start")
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, command := range batch {
			if command != nil {
				command()
			}
		}
	}
	if added != "/tmp/Test File.pdf" {
		t.Fatalf("added path = %q", added)
	}
}

func TestAddedMessageReportsFolderCount(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	updated, _ := m.Update(addedMsg{count: 3})
	if message := updated.(Model).message; message != "3 documents imported" {
		t.Fatalf("message = %q", message)
	}
}

func TestLibraryRefreshPreservesImportResult(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.width, m.height = 100, 28
	updated, _ := m.Update(addedMsg{count: 1})
	m = updated.(Model)
	updated, _ = m.Update(loadedMsg{docs: []domain.Document{{OriginalName: "receipt.png", Category: "receipts"}}})
	m = updated.(Model)
	if message := m.message; message != "1 document imported" {
		t.Fatalf("message = %q", message)
	}
	view := m.View()
	for _, value := range []string{"1 document imported", "enter details", "a add", "s models"} {
		if !strings.Contains(view, value) {
			t.Fatalf("view does not contain %q", value)
		}
	}
}

func TestPartialFolderImportReportsCompletedDocuments(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{
		Config: config.Config{Provider: "openai", Model: "test-model"},
		List:   func(context.Context) ([]domain.Document, error) { return nil, nil },
	})
	updated, cmd := m.Update(addedMsg{count: 2, err: context.DeadlineExceeded})
	if message := updated.(Model).message; message != "2 documents filed; context deadline exceeded" {
		t.Fatalf("message = %q", message)
	}
	if cmd == nil {
		t.Fatal("expected library refresh after partial import")
	}
}

func TestSettingsSwitchProviderClearsIncompatibleModel(t *testing.T) {
	t.Parallel()
	var saved config.Config
	m := New(Dependencies{
		Config: config.Config{Provider: "openrouter", Model: "router-model", LibraryPath: "/library"},
		SaveConfig: func(cfg config.Config) error {
			saved = cfg
			return nil
		},
	})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)

	if saved.Provider != "" {
		t.Fatalf("unexpected saved config = %+v", saved)
	}
	if m.settings.Provider != "openai" || m.settings.Model != "" || m.message != "Choose a model before saving" {
		t.Fatalf("mode = %v, message = %q", m.mode, m.message)
	}
}

func TestModelPickerLoadsFiltersAndSelectsModel(t *testing.T) {
	t.Parallel()
	var requestedProvider string
	var saved config.Config
	m := New(Dependencies{
		Config: config.Config{Provider: "openai", Model: "old-model"},
		SaveConfig: func(cfg config.Config) error {
			saved = cfg
			return nil
		},
		Models: func(_ context.Context, providerName string) ([]provider.Model, error) {
			requestedProvider = providerName
			return []provider.Model{
				{ID: "gpt-large", Name: "Large"},
				{ID: "gpt-mini", Name: "Mini"},
			}, nil
		},
	})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = updated.(Model)
	if !m.busy || cmd == nil {
		t.Fatal("expected model loading to start")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatal("expected model-loading batch")
	}
	updated, _ = m.Update(batch[0]())
	m = updated.(Model)
	if requestedProvider != "openai" || m.mode != modelPicker || len(m.models) != 2 || m.models[0].Name != "Large" {
		t.Fatalf("provider = %q, mode = %v, models = %+v", requestedProvider, m.mode, m.models)
	}

	for _, value := range []rune("mini") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != browse || m.settings.Model != "gpt-mini" || saved.Model != "gpt-mini" {
		t.Fatalf("mode = %v, model = %q, saved = %+v", m.mode, m.settings.Model, saved)
	}
}

func TestSearchFiltersDocuments(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.docs = []domain.Document{
		{OriginalName: "lease.pdf", Category: "housing"},
		{OriginalName: "receipt.jpg", Category: "receipts"},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	for _, value := range []rune("hous") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}})
		m = updated.(Model)
	}
	visible := m.visibleDocuments()
	if len(visible) != 1 || visible[0].OriginalName != "lease.pdf" {
		t.Fatalf("visible documents = %+v", visible)
	}
}

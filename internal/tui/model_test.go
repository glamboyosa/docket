package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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
		"rental-agreement.pdf", "HOUSING", "94%",
		"JEV CLASSIFICATION", "JEV OUTPUT", "0.8 / 3 · Personal", "1.6 / 3 · Time-sensitive", "Needs action", "82%",
		"DOCKET GUIDANCE", "Keep private", "Likely required · 82%", "Act soon; check exact deadline",
		"housing · soon", "↑/↓ select", "a browse", "s models",
	} {
		if !strings.Contains(view, value) {
			t.Fatalf("view does not contain %q", value)
		}
	}
	if lines := strings.Count(view, "\n") + 1; lines > m.height {
		t.Fatalf("view is %d lines tall in a %d-line terminal", lines, m.height)
	}
}

func TestGuidanceEscalatesAmbiguousActionAndCategory(t *testing.T) {
	t.Parallel()
	doc := domain.Document{
		Category: "other", CategoryConfidence: .42, Review: true,
		Sensitivity: 2.7, Urgency: 1.8, NeedsActionProbability: .52,
	}
	view := strings.Join(guidanceRows(doc, 60), "\n")
	for _, value := range []string{"2.7 / 3 · Highly sensitive", "1.8 / 3 · Time-sensitive", "52%", "Check for a request or deadline", "Secure carefully", "Unclear · 52%", "Required · verify category"} {
		if !strings.Contains(view, value) {
			t.Fatalf("guidance does not contain %q: %s", value, view)
		}
	}
}

func TestScoreGuidanceUsesNearestRubricLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		score            float64
		sensitivityLevel string
		urgencyLevel     string
		handling         string
	}{
		{name: "level zero", score: .49, sensitivityLevel: "Routine", urgencyLevel: "No deadline", handling: "Normal handling"},
		{name: "level one", score: .5, sensitivityLevel: "Personal", urgencyLevel: "Action eventually", handling: "Keep private"},
		{name: "level two", score: 1.5, sensitivityLevel: "Confidential", urgencyLevel: "Time-sensitive", handling: "Limit sharing"},
		{name: "level three", score: 2.5, sensitivityLevel: "Highly sensitive", urgencyLevel: "Immediate", handling: "Secure carefully"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sensitivityLevel(test.score); got != test.sensitivityLevel {
				t.Fatalf("sensitivityLevel(%v) = %q", test.score, got)
			}
			if got := urgencyLevel(test.score); got != test.urgencyLevel {
				t.Fatalf("urgencyLevel(%v) = %q", test.score, got)
			}
			if got := handlingGuidance(test.score); got != test.handling {
				t.Fatalf("handlingGuidance(%v) = %q", test.score, got)
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
		Provider: "openai", Model: "gpt-test",
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != inspectDocument {
		t.Fatalf("mode = %v", m.mode)
	}
	view := m.View()
	for _, value := range []string{"Document details", "2.8 / 3 · Immediate", "Act now; check exact deadline", "openai", "gpt-test"} {
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
	updated, _ = m.Update(directoryMsg{path: "/tmp", entries: []fileEntry{}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
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

func TestAddOpensInTerminalFileBrowser(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "receipts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lease.pdf"), []byte("lease"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "archive.zip"), []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New(Dependencies{
		Config:     config.Config{Provider: "openai", Model: "test-model"},
		BrowsePath: root,
	})
	m.browserLoading = false
	m.width, m.height = 100, 28
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if m.mode != browseFiles || !m.busy || cmd == nil {
		t.Fatalf("mode = %v, busy = %v, cmd = %v", m.mode, m.busy, cmd)
	}

	updated, _ = m.Update(cmd())
	m = updated.(Model)
	view := m.View()
	for _, value := range []string{"BROWSE IN DOCKET", "receipts", "lease.pdf", "enter open/import", "o system files", "O system folder", "p paste/drag path"} {
		if !strings.Contains(view, value) {
			t.Fatalf("file browser does not contain %q: %s", value, view)
		}
	}
	if strings.Contains(view, "archive.zip") {
		t.Fatalf("file browser includes unsupported file: %s", view)
	}
}

func TestInitPreloadsFileBrowser(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "lease.pdf"), []byte("lease"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New(Dependencies{
		Config:     config.Config{Provider: "openai", Model: "test-model"},
		BrowsePath: root,
		List:       func(context.Context) ([]domain.Document, error) { return nil, nil },
	})
	commands, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected startup commands to be batched")
	}
	for _, command := range commands {
		if message := command(); message != nil {
			updated, _ := m.Update(message)
			m = updated.(Model)
		}
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if m.mode != browseFiles || m.busy || cmd != nil {
		t.Fatalf("mode = %v, busy = %v, cmd = %v", m.mode, m.busy, cmd)
	}
	if len(m.browserEntries) != 1 || m.browserEntries[0].name != "lease.pdf" {
		t.Fatalf("browser entries = %+v", m.browserEntries)
	}
}

func TestFileBrowserImportsHighlightedFile(t *testing.T) {
	t.Parallel()
	var added string
	m := New(Dependencies{
		Config: config.Config{Provider: "openai", Model: "test-model"},
		Add: func(_ context.Context, path string) (int, error) {
			added = path
			return 1, nil
		},
	})
	m.mode = browseFiles
	m.browserEntries = []fileEntry{{name: "lease.pdf", path: "/tmp/lease.pdf"}}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != browse || !m.busy || cmd == nil {
		t.Fatalf("mode = %v, busy = %v, cmd = %v", m.mode, m.busy, cmd)
	}
	commands := cmd().(tea.BatchMsg)
	updated, _ = m.Update(commands[0]())
	m = updated.(Model)
	if added != "/tmp/lease.pdf" || m.message != "1 document imported" {
		t.Fatalf("added = %q, message = %q", added, m.message)
	}
}

func TestFileBrowserOpensSelectedDirectory(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.mode = browseFiles
	m.browserEntries = []fileEntry{{name: "receipts", path: "/tmp/receipts", isDir: true}}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != browseFiles || !m.busy || cmd == nil {
		t.Fatalf("mode = %v, busy = %v, cmd = %v", m.mode, m.busy, cmd)
	}
	message := cmd()
	if got := message.(directoryMsg).path; got != "/tmp/receipts" {
		t.Fatalf("directory path = %q", got)
	}
}

func TestRightArrowOpensSelectedDirectory(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.mode = browseFiles
	m.browserEntries = []fileEntry{{name: "receipts", path: "/tmp/receipts", isDir: true}}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	if m.mode != browseFiles || !m.busy || cmd == nil {
		t.Fatalf("mode = %v, busy = %v, cmd = %v", m.mode, m.busy, cmd)
	}
	message := cmd()
	if got := message.(directoryMsg).path; got != "/tmp/receipts" {
		t.Fatalf("directory path = %q", got)
	}
}

func TestFileBrowserShowsDirectoryErrors(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.width, m.height, m.mode = 100, 28, browseFiles
	m.browserPath = "/tmp"
	updated, _ := m.Update(directoryMsg{err: os.ErrPermission})
	view := updated.(Model).View()
	if !strings.Contains(view, "permission denied") {
		t.Fatalf("file browser does not show the directory error: %s", view)
	}
}

func TestGraphicalFilePickerImportsEverySelection(t *testing.T) {
	t.Parallel()
	var added []string
	m := New(Dependencies{
		Config: config.Config{Provider: "openai", Model: "test-model"},
		PickFiles: func(context.Context) ([]string, error) {
			return []string{"/tmp/lease.pdf", "/tmp/photo.jpg"}, nil
		},
		Add: func(_ context.Context, path string) (int, error) {
			added = append(added, path)
			return 1, nil
		},
	})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	if !m.busy || m.message != "Opening file picker…" || cmd == nil {
		t.Fatalf("busy = %v, message = %q", m.busy, m.message)
	}

	commands := cmd().(tea.BatchMsg)
	updated, cmd = m.Update(commands[0]())
	m = updated.(Model)
	if !m.busy || m.message != "Importing 2 documents…" || cmd == nil {
		t.Fatalf("busy = %v, message = %q", m.busy, m.message)
	}

	commands = cmd().(tea.BatchMsg)
	updated, _ = m.Update(commands[0]())
	m = updated.(Model)
	if !slices.Equal(added, []string{"/tmp/lease.pdf", "/tmp/photo.jpg"}) {
		t.Fatalf("added = %q", added)
	}
	if m.message != "2 documents imported" {
		t.Fatalf("message = %q", m.message)
	}
}

func TestGraphicalPickerCancelReturnsToLibrary(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{
		Config:    config.Config{Provider: "openai", Model: "test-model"},
		PickFiles: func(context.Context) ([]string, error) { return nil, nil },
	})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	commands := cmd().(tea.BatchMsg)
	updated, cmd = m.Update(commands[0]())
	m = updated.(Model)
	if m.busy || m.message != "Selection cancelled" || cmd != nil {
		t.Fatalf("busy = %v, message = %q, cmd = %v", m.busy, m.message, cmd)
	}
}

func TestGraphicalFolderPickerUsesUppercaseO(t *testing.T) {
	t.Parallel()
	var picked bool
	m := New(Dependencies{
		Config: config.Config{Provider: "openai", Model: "test-model"},
		PickFolder: func(context.Context) (string, error) {
			picked = true
			return "/tmp/documents", nil
		},
		Add: func(context.Context, string) (int, error) { return 3, nil },
	})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	commands := cmd().(tea.BatchMsg)
	updated, _ = updated.(Model).Update(commands[0]())
	if !picked || !updated.(Model).busy {
		t.Fatalf("picked = %v, busy = %v", picked, updated.(Model).busy)
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

func TestLibraryRefreshShowsUpdatedDocumentCount(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.width, m.height = 100, 28
	m.docs = []domain.Document{{OriginalName: "lease.pdf", Category: "housing"}}
	updated, _ := m.Update(addedMsg{count: 1})
	m = updated.(Model)
	updated, _ = m.Update(loadedMsg{docs: []domain.Document{
		{OriginalName: "receipt.png", Category: "receipts"},
		{OriginalName: "lease.pdf", Category: "housing"},
	}})
	m = updated.(Model)
	if message := m.message; message != "2 documents in library" {
		t.Fatalf("message = %q", message)
	}
	view := m.View()
	for _, value := range []string{"DOCUMENTS  2", "2 documents in library", "enter details", "a browse", "s models"} {
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

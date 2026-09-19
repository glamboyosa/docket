package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/glamboyosa/docket/internal/config"
	"github.com/glamboyosa/docket/internal/domain"
)

func TestLibraryViewShowsClassification(t *testing.T) {
	t.Parallel()
	m := New(Dependencies{Config: config.Config{Provider: "openai", Model: "test-model"}})
	m.width, m.height = 100, 28
	updated, _ := m.Update(loadedMsg{docs: []domain.Document{{
		OriginalName: "rental-agreement.pdf", Category: "housing", CategoryConfidence: .94,
		Status: domain.StatusFiled, Provider: "openai", Model: "test-model", LibraryPath: "/library/housing/rental-agreement.pdf",
	}}})
	view := updated.(Model).View()
	for _, value := range []string{"rental-agreement.pdf", "HOUSING", "94%", "openai"} {
		if !strings.Contains(view, value) {
			t.Fatalf("view does not contain %q", value)
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
	for _, value := range []rune("/tmp/Test\\ File.pdf") {
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
	if message := updated.(Model).message; message != "3 documents copied, classified, and filed" {
		t.Fatalf("message = %q", message)
	}
}

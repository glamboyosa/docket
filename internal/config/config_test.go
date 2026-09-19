package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DOCKET_HOME", home)
	want := Config{Provider: "openai", Model: "gpt-test", LibraryPath: filepath.Join(home, "library")}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

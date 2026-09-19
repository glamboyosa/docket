package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultUsesProviderRouterInsteadOfVersionedModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "openrouter" || cfg.Model != "openrouter/auto" {
		t.Fatalf("default config = %+v", cfg)
	}
}

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

func TestLoadMigratesFormerGeminiDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DOCKET_HOME", home)
	data := []byte(`{"provider":"openrouter","model":"google/gemini-2.5-flash","library_path":"/library"}`)
	if err := os.WriteFile(filepath.Join(home, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "openrouter/auto" {
		t.Fatalf("model = %q", cfg.Model)
	}
}

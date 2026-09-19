package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportCopiesWithoutChangingSource(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "statement.txt")
	if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, hash, err := (Library{Root: root}).Import(source)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("expected document hash")
	}
	copied, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != "original" {
		t.Fatalf("copy = %q, want original", copied)
	}
	if err := os.WriteFile(path, []byte("changed copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(original) != "original" {
		t.Fatalf("source changed to %q", original)
	}
}

func TestFileUsesKnownCategoryAndKeepsExtension(t *testing.T) {
	t.Parallel()
	library := Library{Root: t.TempDir()}
	inbox := filepath.Join(library.Root, "inbox")
	if err := os.MkdirAll(inbox, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(inbox, "notice.pdf")
	if err := os.WriteFile(source, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := library.File(source, "legal")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(library.Root, "legal") || filepath.Ext(path) != ".pdf" {
		t.Fatalf("unexpected filed path %q", path)
	}
}

package files

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverFindsSupportedDocumentsRecursively(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(root, "invoice.PDF"):  "pdf",
		filepath.Join(root, "notes.csv"):    "csv",
		filepath.Join(nested, "letter.txt"): "text",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "linked.md")
	if err := os.WriteFile(outside, []byte("linked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.md")); err != nil {
		t.Fatal(err)
	}

	paths, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "invoice.PDF"), filepath.Join(nested, "letter.txt")}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %q, want %q", paths, want)
	}
}

func TestDiscoverRejectsDirectoryWithoutDocuments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.csv"), []byte("value"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Discover(root)
	if err == nil || err.Error() != "directory contains no supported documents" {
		t.Fatalf("error = %v", err)
	}
}

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

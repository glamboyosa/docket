package picker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPickFilesDarwinReturnsMultiplePaths(t *testing.T) {
	t.Parallel()

	paths, err := pickFiles(context.Background(), "darwin", nil, func(_ context.Context, name string, args ...string) (string, error) {
		if name != "osascript" {
			t.Fatalf("command = %q", name)
		}
		if script := strings.Join(args, " "); !strings.Contains(script, "multiple selections allowed") {
			t.Fatalf("script = %q", script)
		}
		return "/Users/osa/Downloads/lease.pdf\n/Users/osa/Downloads/photo one.jpg\n", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/Users/osa/Downloads/lease.pdf", "/Users/osa/Downloads/photo one.jpg"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %q", paths)
	}
}

func TestPickFolderTreatsEmptySelectionAsCancelled(t *testing.T) {
	t.Parallel()

	_, err := pickFolder(context.Background(), "windows", nil, func(context.Context, string, ...string) (string, error) {
		return "", nil
	})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("error = %v", err)
	}
}

func TestPickFilesLinuxFallsBackToKDialog(t *testing.T) {
	t.Parallel()

	lookup := func(name string) (string, error) {
		if name == "kdialog" {
			return "/usr/bin/kdialog", nil
		}
		return "", errors.New("not found")
	}
	paths, err := pickFiles(context.Background(), "linux", lookup, func(_ context.Context, name string, _ ...string) (string, error) {
		if name != "/usr/bin/kdialog" {
			t.Fatalf("command = %q", name)
		}
		return "/home/osa/one.pdf\n/home/osa/two.pdf\n", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %q", paths)
	}
}

func TestPickFilesLinuxExplainsMissingDialog(t *testing.T) {
	t.Parallel()

	_, err := pickFiles(context.Background(), "linux", func(string) (string, error) {
		return "", errors.New("not found")
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "Zenity or KDialog") {
		t.Fatalf("error = %v", err)
	}
}

func TestPickFilesReturnsDialogFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("dialog failed")
	_, err := pickFiles(context.Background(), "darwin", nil, func(context.Context, string, ...string) (string, error) {
		return "", want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}

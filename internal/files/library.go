package files

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxDocumentSize = 25 << 20

var supported = map[string]bool{
	".pdf": true, ".png": true, ".jpg": true, ".jpeg": true, ".webp": true,
	".txt": true, ".md": true,
}

type Library struct{ Root string }

func (l Library) Import(source string) (path, hash string, err error) {
	info, err := os.Stat(source)
	if err != nil {
		return "", "", fmt.Errorf("inspect document: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("%s is not a regular file", source)
	}
	if info.Size() > MaxDocumentSize {
		return "", "", fmt.Errorf("document exceeds the 25 MB limit")
	}
	ext := strings.ToLower(filepath.Ext(source))
	if !supported[ext] {
		return "", "", fmt.Errorf("unsupported document type %q", ext)
	}

	in, err := os.Open(source)
	if err != nil {
		return "", "", fmt.Errorf("open document: %w", err)
	}
	defer in.Close()
	h := sha256.New()
	if _, err := io.Copy(h, in); err != nil {
		return "", "", fmt.Errorf("hash document: %w", err)
	}
	hash = hex.EncodeToString(h.Sum(nil))
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return "", "", fmt.Errorf("rewind document: %w", err)
	}

	inbox := filepath.Join(l.Root, "inbox")
	if err := os.MkdirAll(inbox, 0o700); err != nil {
		return "", "", fmt.Errorf("create inbox: %w", err)
	}
	path = availablePath(inbox, filepath.Base(source), hash[:8])
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", fmt.Errorf("create library copy: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(path)
		return "", "", fmt.Errorf("copy document: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", "", fmt.Errorf("close library copy: %w", err)
	}
	return path, hash, nil
}

func (l Library) File(path, category string) (string, error) {
	if !supportedCategory(category) {
		category = "other"
	}
	dir := filepath.Join(l.Root, category)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create category directory: %w", err)
	}
	destination := availablePath(dir, filepath.Base(path), "copy")
	if err := os.Rename(path, destination); err != nil {
		return "", fmt.Errorf("file document: %w", err)
	}
	return destination, nil
}

func availablePath(dir, name, suffix string) string {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, suffix, ext))
}

func supportedCategory(category string) bool {
	for _, value := range []string{"tax", "legal", "financial", "medical", "identity", "insurance", "employment", "education", "housing", "receipts", "correspondence", "other"} {
		if category == value {
			return true
		}
	}
	return false
}

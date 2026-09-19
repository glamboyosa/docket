package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/glamboyosa/docket/internal/app"
	"github.com/glamboyosa/docket/internal/config"
	"github.com/glamboyosa/docket/internal/domain"
	"github.com/glamboyosa/docket/internal/files"
	"github.com/glamboyosa/docket/internal/provider"
	"github.com/glamboyosa/docket/internal/store"
	"github.com/glamboyosa/docket/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "docket:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runTUI()
	}
	switch args[0] {
	case "add":
		return add(args[1:])
	case "auth":
		return auth(args[1:])
	case "config":
		return configure(args[1:])
	case "models":
		return listModels(args[1:])
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q; run docket help", args[0])
	}
}

func runTUI() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	model := tui.New(tui.Dependencies{
		Config: cfg,
		List:   s.List,
		Add: func(ctx context.Context, path string) (int, error) {
			docs, err := processPath(ctx, s, path)
			return len(docs), err
		},
		SaveConfig: config.Save,
		Models: func(ctx context.Context, providerName string) ([]provider.Model, error) {
			apiKey, err := config.Secret(providerName)
			if err != nil {
				return nil, err
			}
			return provider.Models(ctx, providerName, apiKey)
		},
	})
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func add(paths []string) error {
	if len(paths) == 0 {
		return errors.New("usage: docket add <path> [path…] or docket add -")
	}
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	if len(paths) == 1 && paths[0] == "-" {
		path, cleanup, err := documentFromStdin(os.Stdin)
		if err != nil {
			return err
		}
		defer cleanup()
		paths = []string{path}
	}
	for _, path := range paths {
		docs, err := processPath(context.Background(), s, path)
		for _, doc := range docs {
			fmt.Printf("%s  %s  %.0f%%\n", doc.Category, doc.OriginalName, doc.CategoryConfidence*100)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func processPath(ctx context.Context, s *store.Store, path string) ([]*domain.Document, error) {
	paths, err := files.Discover(path)
	if err != nil {
		return nil, err
	}
	docs := make([]*domain.Document, 0, len(paths))
	for _, candidate := range paths {
		doc, err := process(ctx, s, candidate)
		if err != nil {
			return docs, fmt.Errorf("process %s: %w", candidate, err)
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func process(ctx context.Context, s *store.Store, path string) (*domain.Document, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var extractionKey string
	if provider.RequiresRemoteExtraction(path) {
		extractionKey, err = config.Secret(cfg.Provider)
		if err != nil {
			return nil, err
		}
	}
	jevKey, err := config.Secret("typesafe")
	if err != nil {
		return nil, err
	}
	processor := app.Processor{
		Store: s, Library: files.Library{Root: cfg.LibraryPath},
		Extractor:  provider.RemoteExtractor{Provider: cfg.Provider, Model: cfg.Model, APIKey: extractionKey},
		Classifier: provider.Jev{APIKey: jevKey}, Provider: cfg.Provider, Model: cfg.Model,
	}
	return processor.Add(ctx, path)
}

func openStore() (*store.Store, error) {
	dir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	return store.Open(filepath.Join(dir, "docket.db"))
}

func auth(args []string) error {
	if len(args) == 0 || args[0] == "status" {
		for _, name := range config.Providers() {
			fmt.Printf("%-12s %s\n", name, config.SecretSource(name))
		}
		return nil
	}
	if len(args) != 2 || (args[0] != "set" && args[0] != "forget") {
		return errors.New("usage: docket auth set|forget typesafe|openrouter|openai")
	}
	providerName := args[1]
	if args[0] == "forget" {
		return config.DeleteSecret(providerName)
	}
	fmt.Fprintf(os.Stderr, "%s key: ", providerName)
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("read credential: %w", err)
	}
	if err := config.SetSecret(providerName, string(value)); err != nil {
		return fmt.Errorf("store credential: %w", err)
	}
	fmt.Println("Saved to the OS keychain.")
	return nil
}

func configure(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Printf("provider     %s\nmodel        %s\nlibrary      %s\n", cfg.Provider, cfg.Model, cfg.LibraryPath)
		return nil
	}
	if len(args) != 2 {
		return errors.New("usage: docket config provider|model|library <value>")
	}
	switch args[0] {
	case "provider":
		if args[1] != "openrouter" && args[1] != "openai" {
			return errors.New("provider must be openrouter or openai")
		}
		cfg.Provider = args[1]
	case "model":
		cfg.Model = args[1]
	case "library":
		cfg.LibraryPath = args[1]
	default:
		return errors.New("setting must be provider, model, or library")
	}
	return config.Save(cfg)
}

func listModels(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	providerName := cfg.Provider
	if len(args) > 1 {
		return errors.New("usage: docket models [openrouter|openai]")
	}
	if len(args) == 1 {
		providerName = args[0]
	}
	apiKey, err := config.Secret(providerName)
	if err != nil {
		return err
	}
	models, err := provider.Models(context.Background(), providerName, apiKey)
	if err != nil {
		return err
	}
	for _, model := range models {
		fmt.Printf("%-48s %s\n", model.ID, model.Name)
	}
	return nil
}

func documentFromStdin(reader io.Reader) (string, func(), error) {
	data, err := io.ReadAll(bufio.NewReader(reader))
	if err != nil {
		return "", func() {}, fmt.Errorf("read stdin: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", func() {}, errors.New("stdin is empty")
	}
	file, err := os.CreateTemp("", "docket-paste-*.txt")
	if err != nil {
		return "", func() {}, fmt.Errorf("create pasted document: %w", err)
	}
	cleanup := func() {
		if err := os.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "docket: remove temporary paste:", err)
		}
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write pasted document: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close pasted document: %w", err)
	}
	return file.Name(), cleanup, nil
}

func printHelp() {
	fmt.Println(`Docket copies, reads, classifies, and files your documents.

Usage:
  docket                         Open the terminal interface
  docket add <path> [path…]      Import files or directories
  docket add -                   Import pasted text from stdin
  docket auth status             Show credential sources
  docket auth set <provider>     Save a key to the OS keychain
  docket auth forget <provider>  Remove a key from the OS keychain
  docket config                  Show extraction settings
  docket config <key> <value>    Set provider, model, or library
  docket models [provider]       List live PDF and image extraction models`)
}

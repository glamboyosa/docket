package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glamboyosa/docket/internal/domain"
	"github.com/glamboyosa/docket/internal/files"
	"github.com/glamboyosa/docket/internal/provider"
	"github.com/glamboyosa/docket/internal/store"
)

type Processor struct {
	Store      *store.Store
	Library    files.Library
	Extractor  provider.Extractor
	Classifier provider.Classifier
	Provider   string
	Model      string
	OnStatus   func(domain.Status)
}

func (p *Processor) Add(ctx context.Context, source string) (*domain.Document, error) {
	path, hash, err := p.Library.Import(source)
	if err != nil {
		return nil, err
	}
	existing, err := p.Store.ByHash(ctx, hash)
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	if existing != nil {
		os.Remove(path)
		return existing, nil
	}
	doc := &domain.Document{
		SHA256: hash, OriginalName: filepath.Base(source), SourcePath: source, LibraryPath: path,
		Status: domain.StatusQueued, Provider: p.Provider, Model: p.Model,
	}
	if err := p.Store.Create(ctx, doc); err != nil {
		os.Remove(path)
		return nil, err
	}
	if err := p.Process(ctx, doc); err != nil {
		return doc, err
	}
	return p.Store.ByID(ctx, doc.ID)
}

func (p *Processor) Process(ctx context.Context, doc *domain.Document) error {
	p.status(ctx, doc.ID, domain.StatusExtracting, "")
	text, err := p.Extractor.Extract(ctx, doc.LibraryPath)
	if err != nil {
		return p.fail(ctx, doc.ID, fmt.Errorf("extract text: %w", err))
	}
	p.status(ctx, doc.ID, domain.StatusClassifying, "")
	classification, err := p.Classifier.Classify(ctx, text)
	if err != nil {
		return p.fail(ctx, doc.ID, fmt.Errorf("classify document: %w", err))
	}
	path, err := p.Library.File(doc.LibraryPath, classification.Category)
	if err != nil {
		return p.fail(ctx, doc.ID, err)
	}
	if err := p.Store.Complete(ctx, doc.ID, path, classification); err != nil {
		return fmt.Errorf("save classification: %w", err)
	}
	if p.OnStatus != nil {
		p.OnStatus(domain.StatusFiled)
	}
	return nil
}

func (p *Processor) status(ctx context.Context, id int64, status domain.Status, message string) {
	_ = p.Store.UpdateStatus(ctx, id, status, message)
	if p.OnStatus != nil {
		p.OnStatus(status)
	}
}

func (p *Processor) fail(ctx context.Context, id int64, err error) error {
	p.status(ctx, id, domain.StatusFailed, err.Error())
	return err
}

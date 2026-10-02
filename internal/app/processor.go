package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

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

func (p *Processor) Process(ctx context.Context, doc *domain.Document) (processErr error) {
	ctx, span := otel.Tracer("docket").Start(ctx, "docket.process")
	defer func() {
		if processErr != nil {
			span.SetStatus(codes.Error, "document processing failed")
		}
		span.End()
	}()
	span.SetAttributes(attribute.String("langfuse.observation.type", "span"))
	if environment := os.Getenv("LANGFUSE_TRACING_ENVIRONMENT"); environment != "" {
		span.SetAttributes(attribute.String("langfuse.environment", environment))
	}
	if traceFile := os.Getenv("DOCKET_EVAL_TRACE_ID_FILE"); traceFile != "" {
		if err := os.WriteFile(traceFile, []byte(span.SpanContext().TraceID().String()), 0o600); err != nil {
			return fmt.Errorf("write evaluation trace ID: %w", err)
		}
	}
	if err := p.status(ctx, doc.ID, domain.StatusExtracting, ""); err != nil {
		return err
	}
	extractCtx, extraction := otel.Tracer("docket").Start(ctx, "docket.extract")
	if provider.RequiresRemoteExtraction(doc.LibraryPath) {
		extraction.SetAttributes(attribute.String("langfuse.observation.type", "generation"), attribute.String("gen_ai.request.model", p.Model), attribute.String("gen_ai.system", p.Provider))
	}
	text, err := p.Extractor.Extract(extractCtx, doc.LibraryPath)
	if err != nil {
		extraction.SetStatus(codes.Error, "extraction failed")
	}
	extraction.End()
	if err != nil {
		return p.fail(ctx, doc.ID, fmt.Errorf("extract text: %w", err))
	}
	if err := p.status(ctx, doc.ID, domain.StatusClassifying, ""); err != nil {
		return err
	}
	classifyCtx, classificationSpan := otel.Tracer("docket").Start(ctx, "docket.classify")
	classificationSpan.SetAttributes(attribute.String("langfuse.observation.type", "generation"), attribute.String("gen_ai.request.model", "jev-latest"), attribute.String("gen_ai.system", "typesafe"))
	classification, err := p.Classifier.Classify(classifyCtx, text)
	if err != nil {
		classificationSpan.SetStatus(codes.Error, "classification failed")
	} else {
		classificationSpan.SetAttributes(attribute.String("docket.category", classification.Category), attribute.Float64("docket.category_confidence", classification.CategoryConfidence), attribute.Bool("docket.review", classification.Review))
	}
	classificationSpan.End()
	if err != nil {
		return p.fail(ctx, doc.ID, fmt.Errorf("classify document: %w", err))
	}
	span.SetAttributes(
		attribute.String("docket.category", classification.Category),
		attribute.Bool("docket.needs_action", classification.NeedsAction),
		attribute.Bool("docket.review", classification.Review),
	)
	span.SetAttributes(attribute.String("langfuse.observation.output", classification.Category))
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

func (p *Processor) status(ctx context.Context, id int64, status domain.Status, message string) error {
	if err := p.Store.UpdateStatus(ctx, id, status, message); err != nil {
		return fmt.Errorf("update document status: %w", err)
	}
	if p.OnStatus != nil {
		p.OnStatus(status)
	}
	return nil
}

func (p *Processor) fail(ctx context.Context, id int64, err error) error {
	return errors.Join(err, p.status(ctx, id, domain.StatusFailed, err.Error()))
}

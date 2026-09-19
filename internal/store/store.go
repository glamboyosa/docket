package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/glamboyosa/docket/internal/domain"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		PRAGMA journal_mode = WAL;
		CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY,
			sha256 TEXT NOT NULL UNIQUE,
			original_name TEXT NOT NULL,
			source_path TEXT NOT NULL,
			library_path TEXT NOT NULL,
			status TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT '',
			category_confidence REAL NOT NULL DEFAULT 0,
			category_probabilities TEXT NOT NULL DEFAULT '{}',
			sensitivity REAL NOT NULL DEFAULT 0,
			urgency REAL NOT NULL DEFAULT 0,
			needs_action INTEGER NOT NULL DEFAULT 0,
			needs_action_probability REAL NOT NULL DEFAULT 0,
			review INTEGER NOT NULL DEFAULT 0,
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS documents_created_at ON documents(created_at DESC);
	`)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

func (s *Store) Create(ctx context.Context, doc *domain.Document) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO documents (sha256, original_name, source_path, library_path, status, provider, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.SHA256, doc.OriginalName, doc.SourcePath, doc.LibraryPath, doc.Status, doc.Provider, doc.Model,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	doc.ID, err = result.LastInsertId()
	doc.CreatedAt, doc.UpdatedAt = now, now
	return err
}

func (s *Store) ByHash(ctx context.Context, hash string) (*domain.Document, error) {
	row := s.db.QueryRowContext(ctx, selectDocument+` WHERE sha256 = ?`, hash)
	doc, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return doc, err
}

func (s *Store) ByID(ctx context.Context, id int64) (*domain.Document, error) {
	doc, err := scanDocument(s.db.QueryRowContext(ctx, selectDocument+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return doc, err
}

func (s *Store) List(ctx context.Context) ([]domain.Document, error) {
	rows, err := s.db.QueryContext(ctx, selectDocument+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()
	var docs []domain.Document
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, *doc)
	}
	return docs, rows.Err()
}

func (s *Store) UpdateStatus(ctx context.Context, id int64, status domain.Status, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE documents SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, message, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

func (s *Store) Complete(ctx context.Context, id int64, path string, c domain.Classification) error {
	probabilities, err := json.Marshal(c.CategoryProbabilities)
	if err != nil {
		return fmt.Errorf("encode probabilities: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE documents SET library_path = ?, status = ?, category = ?, category_confidence = ?,
		category_probabilities = ?, sensitivity = ?, urgency = ?, needs_action = ?, needs_action_probability = ?, review = ?, error = '', updated_at = ?
		WHERE id = ?`, path, domain.StatusFiled, c.Category, c.CategoryConfidence, string(probabilities), c.Sensitivity,
		c.Urgency, c.NeedsAction, c.NeedsActionProbability, c.Review, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

const selectDocument = `SELECT id, sha256, original_name, source_path, library_path, status, category,
	category_confidence, category_probabilities, sensitivity, urgency, needs_action, needs_action_probability, review,
	provider, model, error, created_at, updated_at FROM documents`

type scanner interface{ Scan(...any) error }

func scanDocument(s scanner) (*domain.Document, error) {
	var doc domain.Document
	var probabilities, createdAt, updatedAt string
	var needsAction, review bool
	err := s.Scan(&doc.ID, &doc.SHA256, &doc.OriginalName, &doc.SourcePath, &doc.LibraryPath, &doc.Status,
		&doc.Category, &doc.CategoryConfidence, &probabilities, &doc.Sensitivity, &doc.Urgency, &needsAction, &doc.NeedsActionProbability,
		&review, &doc.Provider, &doc.Model, &doc.Error, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	doc.NeedsAction, doc.Review = needsAction, review
	if err := json.Unmarshal([]byte(probabilities), &doc.CategoryProbabilities); err != nil {
		return nil, fmt.Errorf("decode probabilities: %w", err)
	}
	doc.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	doc.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &doc, nil
}

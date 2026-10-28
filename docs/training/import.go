// Package training contains explicitly enabled, fictional onboarding fixtures.
package training

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"urbanops/internal/assistant"
)

//go:embed scenarios.json sources/*.md
var assets embed.FS

type Document struct {
	Title string `json:"title"`
	File  string `json:"file"`
	URL   string `json:"url"`
}

func Documents() ([]Document, error) {
	var plan struct {
		Documents []Document `json:"documents"`
	}
	err := json.Unmarshal(mustRead("scenarios.json"), &plan)
	return plan.Documents, err
}
func mustRead(name string) []byte {
	b, err := assets.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return b
}

// ImportDocuments deliberately stores source text without requiring Ollama.
// SaveDocument deduplicates identical title/content across restarts and CLI runs.
func ImportDocuments(ctx context.Context, db *pgxpool.Pool) error {
	documents, err := Documents()
	if err != nil {
		return err
	}
	for _, document := range documents {
		content, err := assets.ReadFile(document.File)
		if err != nil {
			return err
		}
		if _, err = assistant.SaveDocument(ctx, db, document.Title, string(content)); err != nil {
			return fmt.Errorf("eğitim kaynağı %s: %w", document.Title, err)
		}
	}
	return nil
}

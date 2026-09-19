package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

type Model struct {
	ID     string
	Name   string
	Images bool
}

func Models(ctx context.Context, provider string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://models.dev/api.json", nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("load Models.dev catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Models.dev catalog returned %s", response.Status)
	}
	var catalog map[string]struct {
		Models map[string]struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Attachment bool   `json:"attachment"`
			Modalities struct {
				Input  []string `json:"input"`
				Output []string `json:"output"`
			} `json:"modalities"`
		} `json:"models"`
	}
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode Models.dev catalog: %w", err)
	}
	entry, ok := catalog[provider]
	if !ok {
		return nil, fmt.Errorf("Models.dev has no %s catalog", provider)
	}
	var models []Model
	for _, item := range entry.Models {
		if !item.Attachment || !contains(item.Modalities.Output, "text") {
			continue
		}
		models = append(models, Model{ID: item.ID, Name: item.Name, Images: contains(item.Modalities.Input, "image")})
	}
	sort.Slice(models, func(i, k int) bool { return models[i].Name < models[k].Name })
	return models, nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

const (
	modelsDevURL  = "https://models.dev/api.json"
	openRouterURL = "https://openrouter.ai/api/v1/models?input_modalities=image,file&output_modalities=text&sort=newest"
	openAIURL     = "https://api.openai.com/v1/models"
)

type Model struct {
	ID          string
	Name        string
	Free        bool
	ReleaseDate string
}

type modelMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Attachment  bool   `json:"attachment"`
	ReleaseDate string `json:"release_date"`
	Modalities  struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
}

type modelsDevCatalog map[string]struct {
	Models map[string]modelMetadata `json:"models"`
}

func Models(ctx context.Context, provider, apiKey string) ([]Model, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	return models(ctx, provider, apiKey, client)
}

func models(ctx context.Context, provider, apiKey string, client *http.Client) ([]Model, error) {
	metadata, err := loadModelsDev(ctx, client, provider)
	if err != nil {
		return nil, err
	}

	switch provider {
	case "openrouter":
		return loadOpenRouterModels(ctx, client, apiKey, metadata)
	case "openai":
		return loadOpenAIModels(ctx, client, apiKey, metadata)
	default:
		return nil, fmt.Errorf("unsupported model provider %q", provider)
	}
}

func loadModelsDev(ctx context.Context, client *http.Client, provider string) (map[string]modelMetadata, error) {
	var catalog modelsDevCatalog
	if err := getJSON(ctx, client, modelsDevURL, "", &catalog); err != nil {
		return nil, fmt.Errorf("load Models.dev catalog: %w", err)
	}
	entry, ok := catalog[provider]
	if !ok {
		return nil, fmt.Errorf("Models.dev has no %s catalog", provider)
	}
	metadata := make(map[string]modelMetadata, len(entry.Models))
	for _, model := range entry.Models {
		metadata[model.ID] = model
	}
	return metadata, nil
}

func loadOpenRouterModels(ctx context.Context, client *http.Client, apiKey string, metadata map[string]modelMetadata) ([]Model, error) {
	var catalog struct {
		Data []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Created      int64  `json:"created"`
			Architecture struct {
				Input  []string `json:"input_modalities"`
				Output []string `json:"output_modalities"`
			} `json:"architecture"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := getJSON(ctx, client, openRouterURL, apiKey, &catalog); err != nil {
		return nil, fmt.Errorf("load OpenRouter models: %w", err)
	}

	models := make([]Model, 0, len(catalog.Data))
	for _, item := range catalog.Data {
		if !contains(item.Architecture.Input, "image") || !contains(item.Architecture.Input, "file") || !contains(item.Architecture.Output, "text") {
			continue
		}
		releaseDate := time.Unix(item.Created, 0).UTC().Format(time.DateOnly)
		if details, ok := metadata[item.ID]; ok && details.ReleaseDate != "" {
			releaseDate = details.ReleaseDate
		}
		models = append(models, Model{
			ID: item.ID, Name: item.Name,
			Free: item.Pricing.Prompt == "0" && item.Pricing.Completion == "0", ReleaseDate: releaseDate,
		})
	}
	sortModels(models)
	return models, nil
}

func loadOpenAIModels(ctx context.Context, client *http.Client, apiKey string, metadata map[string]modelMetadata) ([]Model, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not configured; run docket auth set openai")
	}
	var catalog struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, client, openAIURL, apiKey, &catalog); err != nil {
		return nil, fmt.Errorf("load OpenAI models: %w", err)
	}

	available := make(map[string]bool, len(catalog.Data))
	for _, item := range catalog.Data {
		available[item.ID] = true
	}
	models := make([]Model, 0, len(metadata))
	for _, item := range metadata {
		if !available[item.ID] || !item.Attachment || !contains(item.Modalities.Input, "image") || !contains(item.Modalities.Input, "pdf") || !contains(item.Modalities.Output, "text") {
			continue
		}
		models = append(models, Model{
			ID: item.ID, Name: item.Name,
			ReleaseDate: item.ReleaseDate,
		})
	}
	sortModels(models)
	return models, nil
}

func getJSON(ctx context.Context, client *http.Client, url, apiKey string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("request returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func sortModels(models []Model) {
	sort.SliceStable(models, func(i, j int) bool {
		left, leftErr := time.Parse(time.DateOnly, models[i].ReleaseDate)
		right, rightErr := time.Parse(time.DateOnly, models[j].ReleaseDate)
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.After(right)
		}
		if leftErr == nil && rightErr != nil {
			return true
		}
		if leftErr != nil && rightErr == nil {
			return false
		}
		return models[i].Name < models[j].Name
	})
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

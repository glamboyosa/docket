package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const extractionPrompt = "Transcribe every visible word in this document. Preserve headings, tables, labels, and reading order in plain Markdown. Do not summarize or interpret. Mark unreadable text as [illegible]. Return only the transcription."

type Extractor interface {
	Extract(context.Context, string) (string, error)
}

type RemoteExtractor struct {
	Provider string
	Model    string
	APIKey   string
	Client   *http.Client
}

func (e RemoteExtractor) Extract(ctx context.Context, path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".txt" || ext == ".md" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read text document: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read document copy: %w", err)
	}
	if e.Client == nil {
		e.Client = &http.Client{Timeout: 2 * time.Minute}
	}
	switch e.Provider {
	case "openrouter":
		return e.openRouter(ctx, path, data)
	case "openai":
		return e.openAI(ctx, path, data)
	default:
		return "", fmt.Errorf("unsupported extraction provider %q", e.Provider)
	}
}

func (e RemoteExtractor) openRouter(ctx context.Context, path string, data []byte) (string, error) {
	mime := mimeType(path)
	var documentPart map[string]any
	if strings.HasPrefix(mime, "image/") {
		documentPart = map[string]any{"type": "image_url", "image_url": map[string]string{"url": dataURL(mime, data)}}
	} else {
		documentPart = map[string]any{"type": "file", "file": map[string]string{
			"filename": filepath.Base(path), "file_data": dataURL(mime, data),
		}}
	}
	payload := map[string]any{
		"model": e.Model,
		"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]string{"type": "text", "text": extractionPrompt}, documentPart,
		}}},
	}
	if mime == "application/pdf" {
		payload["plugins"] = []any{map[string]any{"id": "file-parser"}}
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := e.post(ctx, "https://openrouter.ai/api/v1/chat/completions", payload, &response, map[string]string{
		"Authorization": "Bearer " + e.APIKey,
		"X-Title":       "Docket",
	}); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("OpenRouter returned no transcription")
	}
	return messageText(response.Choices[0].Message.Content)
}

func (e RemoteExtractor) openAI(ctx context.Context, path string, data []byte) (string, error) {
	mime := mimeType(path)
	part := map[string]any{"type": "input_file", "filename": filepath.Base(path), "file_data": dataURL(mime, data)}
	if strings.HasPrefix(mime, "image/") {
		part = map[string]any{"type": "input_image", "image_url": dataURL(mime, data), "detail": "high"}
	}
	payload := map[string]any{
		"model": e.Model,
		"input": []any{map[string]any{"role": "user", "content": []any{
			map[string]string{"type": "input_text", "text": extractionPrompt}, part,
		}}},
	}
	var response struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := e.post(ctx, "https://api.openai.com/v1/responses", payload, &response, map[string]string{
		"Authorization": "Bearer " + e.APIKey,
	}); err != nil {
		return "", err
	}
	for _, output := range response.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return strings.TrimSpace(content.Text), nil
			}
		}
	}
	return "", fmt.Errorf("OpenAI returned no transcription")
}

func (e RemoteExtractor) post(ctx context.Context, url string, payload any, result any, headers map[string]string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode extraction request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create extraction request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := e.Client.Do(req)
	if err != nil {
		return fmt.Errorf("send extraction request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("extraction request failed (%d): %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("decode extraction response: %w", err)
	}
	return nil
}

func dataURL(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func mimeType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func messageText(content any) (string, error) {
	if value, ok := content.(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	parts, ok := content.([]any)
	if !ok {
		return "", fmt.Errorf("provider returned an unsupported response")
	}
	var text strings.Builder
	for _, part := range parts {
		item, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := item["text"].(string); ok {
			text.WriteString(value)
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", fmt.Errorf("provider returned no transcription")
	}
	return strings.TrimSpace(text.String()), nil
}

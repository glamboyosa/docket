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

type openRouterRequest struct {
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
	Plugins  []openRouterPlugin  `json:"plugins,omitempty"`
}

type openRouterMessage struct {
	Role    string              `json:"role"`
	Content []openRouterContent `json:"content"`
}

type openRouterContent struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	File     *openRouterFile     `json:"file,omitempty"`
	ImageURL *openRouterImageURL `json:"image_url,omitempty"`
}

type openRouterFile struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"`
}

type openRouterImageURL struct {
	URL string `json:"url"`
}

type openRouterPlugin struct {
	ID string `json:"id"`
}

type openAIRequest struct {
	Model string        `json:"model"`
	Input []openAIInput `json:"input"`
}

type openAIInput struct {
	Role    string          `json:"role"`
	Content []openAIContent `json:"content"`
}

type openAIContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
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
	var documentPart openRouterContent
	if strings.HasPrefix(mime, "image/") {
		documentPart = openRouterContent{Type: "image_url", ImageURL: &openRouterImageURL{URL: dataURL(mime, data)}}
	} else {
		documentPart = openRouterContent{Type: "file", File: &openRouterFile{Filename: filepath.Base(path), FileData: dataURL(mime, data)}}
	}
	payload := openRouterRequest{
		Model: e.Model,
		Messages: []openRouterMessage{{Role: "user", Content: []openRouterContent{
			{Type: "text", Text: extractionPrompt}, documentPart,
		}}},
	}
	if mime == "application/pdf" {
		payload.Plugins = []openRouterPlugin{{ID: "file-parser"}}
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	payloadBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode OpenRouter request: %w", err)
	}
	body, err := e.post(ctx, "https://openrouter.ai/api/v1/chat/completions", payloadBody, map[string]string{
		"Authorization": "Bearer " + e.APIKey,
		"X-Title":       "Docket",
	})
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode extraction response: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("OpenRouter returned no transcription")
	}
	text := strings.TrimSpace(response.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("OpenRouter returned no transcription")
	}
	return text, nil
}

func (e RemoteExtractor) openAI(ctx context.Context, path string, data []byte) (string, error) {
	mime := mimeType(path)
	part := openAIContent{Type: "input_file", Filename: filepath.Base(path), FileData: dataURL(mime, data)}
	if strings.HasPrefix(mime, "image/") {
		part = openAIContent{Type: "input_image", ImageURL: dataURL(mime, data), Detail: "high"}
	}
	payload := openAIRequest{
		Model: e.Model,
		Input: []openAIInput{{Role: "user", Content: []openAIContent{
			{Type: "input_text", Text: extractionPrompt}, part,
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
	payloadBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode OpenAI request: %w", err)
	}
	body, err := e.post(ctx, "https://api.openai.com/v1/responses", payloadBody, map[string]string{
		"Authorization": "Bearer " + e.APIKey,
	})
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode extraction response: %w", err)
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

func (e RemoteExtractor) post(ctx context.Context, url string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create extraction request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send extraction request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("extraction request failed (%d): %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read extraction response: %w", err)
	}
	return data, nil
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

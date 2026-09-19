package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiresRemoteExtraction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{path: "notes.txt", want: false},
		{path: "README.MD", want: false},
		{path: "scan.pdf", want: true},
		{path: "receipt.jpg", want: true},
	}
	for _, test := range tests {
		if got := RequiresRemoteExtraction(test.path); got != test.want {
			t.Errorf("RequiresRemoteExtraction(%q) = %t, want %t", test.path, got, test.want)
		}
	}
}

func TestRemoteExtractorReadsTextWithoutHTTP(t *testing.T) {
	t.Parallel()
	path := writeDocument(t, "notice.md", []byte("# Payment notice"))
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("text extraction made an HTTP request")
		return nil, nil
	})}

	got, err := (RemoteExtractor{Provider: "openai", Client: client}).Extract(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "# Payment notice" {
		t.Fatalf("text = %q", got)
	}
}

func TestOpenAIExtractsImage(t *testing.T) {
	t.Parallel()
	path := writeDocument(t, "receipt.png", []byte("image bytes"))
	client := responseClient(t, func(request *http.Request) string {
		if request.URL.String() != "https://api.openai.com/v1/responses" {
			t.Errorf("URL = %s", request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer openai-secret" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		var payload openAIRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "test-vision" || len(payload.Input) != 1 || len(payload.Input[0].Content) != 2 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		part := payload.Input[0].Content[1]
		if part.Type != "input_image" || part.Detail != "high" || !strings.HasPrefix(part.ImageURL, "data:image/png;base64,") {
			t.Fatalf("unexpected image part: %+v", part)
		}
		return `{"output":[{"content":[{"type":"output_text","text":" Store receipt "}]}]}`
	})

	got, err := (RemoteExtractor{Provider: "openai", Model: "test-vision", APIKey: "openai-secret", Client: client}).Extract(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Store receipt" {
		t.Fatalf("text = %q", got)
	}
}

func TestOpenAIExtractsPDF(t *testing.T) {
	t.Parallel()
	path := writeDocument(t, "form.pdf", []byte("pdf bytes"))
	client := responseClient(t, func(request *http.Request) string {
		var payload openAIRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		part := payload.Input[0].Content[1]
		if part.Type != "input_file" || part.Filename != "form.pdf" || !strings.HasPrefix(part.FileData, "data:application/pdf;base64,") {
			t.Fatalf("unexpected file part: %+v", part)
		}
		return `{"output":[{"content":[{"type":"output_text","text":"Tax form"}]}]}`
	})

	got, err := (RemoteExtractor{Provider: "openai", Model: "test-vision", APIKey: "openai-secret", Client: client}).Extract(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Tax form" {
		t.Fatalf("text = %q", got)
	}
}

func TestOpenRouterExtractsPDFWithFileParser(t *testing.T) {
	t.Parallel()
	path := writeDocument(t, "claim.pdf", []byte("pdf bytes"))
	client := responseClient(t, func(request *http.Request) string {
		if request.URL.String() != "https://openrouter.ai/api/v1/chat/completions" {
			t.Errorf("URL = %s", request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer router-secret" || request.Header.Get("X-Title") != "Docket" {
			t.Errorf("unexpected headers: %v", request.Header)
		}
		var payload openRouterRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Plugins) != 1 || payload.Plugins[0].ID != "file-parser" {
			t.Fatalf("plugins = %+v", payload.Plugins)
		}
		part := payload.Messages[0].Content[1]
		if part.Type != "file" || part.File == nil || part.File.Filename != "claim.pdf" || !strings.HasPrefix(part.File.FileData, "data:application/pdf;base64,") {
			t.Fatalf("unexpected file part: %+v", part)
		}
		return `{"choices":[{"message":{"content":"Health claim"}}]}`
	})

	got, err := (RemoteExtractor{Provider: "openrouter", Model: "test-vision", APIKey: "router-secret", Client: client}).Extract(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Health claim" {
		t.Fatalf("text = %q", got)
	}
}

func TestOpenRouterExtractsImage(t *testing.T) {
	t.Parallel()
	path := writeDocument(t, "letter.webp", []byte("image bytes"))
	client := responseClient(t, func(request *http.Request) string {
		var payload openRouterRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Plugins) != 0 {
			t.Fatalf("plugins = %+v", payload.Plugins)
		}
		part := payload.Messages[0].Content[1]
		if part.Type != "image_url" || part.ImageURL == nil || !strings.HasPrefix(part.ImageURL.URL, "data:image/webp;base64,") {
			t.Fatalf("unexpected image part: %+v", part)
		}
		return `{"choices":[{"message":{"content":"Letter"}}]}`
	})

	got, err := (RemoteExtractor{Provider: "openrouter", Model: "test-vision", APIKey: "router-secret", Client: client}).Extract(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Letter" {
		t.Fatalf("text = %q", got)
	}
}

func TestRemoteExtractorReturnsProviderError(t *testing.T) {
	t.Parallel()
	path := writeDocument(t, "scan.jpg", []byte("image bytes"))
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error":"invalid key"}`)),
			Request:    request,
		}, nil
	})}

	_, err := (RemoteExtractor{Provider: "openai", Model: "test-vision", APIKey: "bad", Client: client}).Extract(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("error = %v", err)
	}
}

func writeDocument(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func responseClient(t *testing.T, response func(*http.Request) string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := response(request)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
}

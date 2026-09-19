package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpenRouterModelsUseLiveCapabilitiesAndNewestFirst(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api.json":
			return jsonResponse(`{"openrouter":{"models":{"current":{"id":"vendor/current","name":"Current","release_date":"2026-09-10","modalities":{"input":["text","image","pdf"],"output":["text"]}}}}}`), nil
		case "/api/v1/models":
			if request.Header.Get("Authorization") != "Bearer router-key" {
				return statusResponse(http.StatusUnauthorized), nil
			}
			return jsonResponse(`{"data":[
				{"id":"vendor/old","name":"Old","created":1704067200,"architecture":{"input_modalities":["text","image","file"],"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}},
				{"id":"vendor/current","name":"Current","created":1704067200,"architecture":{"input_modalities":["text","image","file"],"output_modalities":["text"]},"pricing":{"prompt":"1","completion":"2"}},
				{"id":"vendor/no-pdf","name":"No PDF","created":1800000000,"architecture":{"input_modalities":["text","image"],"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}}
			]}`), nil
		default:
			return statusResponse(http.StatusNotFound), nil
		}
	})}

	got, err := models(context.Background(), "openrouter", "router-key", client)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "vendor/current" || got[1].ID != "vendor/old" {
		t.Fatalf("models = %+v", got)
	}
	if got[0].Free || !got[1].Free {
		t.Fatalf("capabilities = %+v", got)
	}
}

func TestOpenAIModelsIntersectAccountAvailabilityAndCapabilities(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api.json":
			return jsonResponse(`{"openai":{"models":{
				"new":{"id":"gpt-new","name":"GPT New","attachment":true,"release_date":"2026-09-01","modalities":{"input":["text","image","pdf"],"output":["text"]}},
				"old":{"id":"gpt-old","name":"GPT Old","attachment":true,"release_date":"2026-01-01","modalities":{"input":["text","image","pdf"],"output":["text"]}},
				"no-pdf":{"id":"gpt-no-pdf","name":"No PDF","attachment":true,"release_date":"2026-09-15","modalities":{"input":["text","image"],"output":["text"]}},
				"unavailable":{"id":"gpt-unavailable","name":"Unavailable","attachment":true,"release_date":"2026-09-18","modalities":{"input":["text","image","pdf"],"output":["text"]}}
			}}}`), nil
		case "/v1/models":
			if request.Header.Get("Authorization") != "Bearer openai-key" {
				return statusResponse(http.StatusUnauthorized), nil
			}
			return jsonResponse(`{"data":[{"id":"gpt-old"},{"id":"gpt-new"},{"id":"gpt-no-pdf"}]}`), nil
		default:
			return statusResponse(http.StatusNotFound), nil
		}
	})}

	got, err := models(context.Background(), "openai", "openai-key", client)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "gpt-new" || got[1].ID != "gpt-old" {
		t.Fatalf("models = %+v", got)
	}
}

func jsonResponse(body string) *http.Response {
	response := statusResponse(http.StatusOK)
	response.Body = io.NopCloser(strings.NewReader(body))
	return response
}

func statusResponse(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

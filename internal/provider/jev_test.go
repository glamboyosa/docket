package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func jsonClient(t *testing.T, body string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected authorization header")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
}

func TestJevParsesClassification(t *testing.T) {
	t.Parallel()
	client := jsonClient(t, `{"answers":{"category":{"choice":"legal","probabilities":{"legal":0.87,"other":0.13},"confidence":0.87},"sensitivity":{"score":2.1,"confidence":0.8},"urgency":{"score":2.8,"confidence":0.9},"needs_action":{"noul":0.92},"action_state":{"choice":"current","confidence":0.9}}}`)
	got, err := (Jev{APIKey: "secret", BaseURL: "https://example.test", Client: client}).Classify(context.Background(), "A legal notice")
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "legal" || got.Urgency != 2.8 || !got.NeedsAction || got.Review {
		t.Fatalf("unexpected classification: %+v", got)
	}
}

func TestJevFlagsLowConfidenceForReview(t *testing.T) {
	t.Parallel()
	client := jsonClient(t, `{"answers":{"category":{"choice":"other","probabilities":{"other":0.4},"confidence":0.4},"sensitivity":{"score":0},"urgency":{"score":0},"needs_action":{"noul":0.1},"action_state":{"choice":"none","confidence":0.9}}}`)
	got, err := (Jev{APIKey: "secret", BaseURL: "https://example.test", Client: client}).Classify(context.Background(), "Unknown")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Review {
		t.Fatal("expected low-confidence result to require review")
	}
}

func TestJevKeepsActionReviewSeparateFromCategory(t *testing.T) {
	t.Parallel()
	client := jsonClient(t, `{"answers":{"category":{"choice":"housing","probabilities":{"housing":0.93},"confidence":0.93},"sensitivity":{"score":1},"urgency":{"score":2},"needs_action":{"noul":0.52},"action_state":{"choice":"unclear","probabilities":{"unclear":0.8},"confidence":0.8}}}`)
	got, err := (Jev{APIKey: "secret", BaseURL: "https://example.test", Client: client}).Classify(context.Background(), "An old housing payment notice")
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "housing" || got.Review || got.NeedsAction || !got.ActionReview || got.ActionAssessment != "unclear" || got.ActionConfidence != .8 {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}

func TestJevDistinguishesOutstandingCompletedAndQuotedRequests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, text, assessment string
		probability            float64
		needsAction, review    bool
	}{
		{"old overdue notice", "Payment was due last year; no outcome is stated", "unclear", .52, false, true},
		{"paid receipt", "Payment received in full", "completed", .1, false, false},
		{"quoted request", "The letter quotes a previous request to someone else", "quoted", .1, false, false},
		{"current request", "Please sign and return this form", "current", .9, true, false},
		{"conflicting signals", "Payment received in full", "completed", .9, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answers := map[string]any{
				"category":     map[string]any{"choice": "housing", "confidence": .9},
				"sensitivity":  map[string]any{"score": 1},
				"urgency":      map[string]any{"score": 1},
				"needs_action": map[string]any{"noul": test.probability},
				"action_state": map[string]any{"choice": test.assessment, "confidence": .9},
			}
			body, err := json.Marshal(map[string]any{"answers": answers})
			if err != nil {
				t.Fatal(err)
			}
			got, err := (Jev{APIKey: "secret", BaseURL: "https://example.test", Client: jsonClient(t, string(body))}).Classify(context.Background(), test.text)
			if err != nil {
				t.Fatal(err)
			}
			if got.Category != "housing" || got.Review || got.ActionAssessment != test.assessment || got.NeedsAction != test.needsAction || got.ActionReview != test.review {
				t.Fatalf("unexpected classification: %+v", got)
			}
		})
	}
}

func TestJevPrioritizesDomainOverBillingFormat(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload jevRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(payload.Questions.Category.Instructions, "Prefer a domain-specific category") {
			t.Fatalf("category instructions = %q", payload.Questions.Category.Instructions)
		}
		if !strings.Contains(payload.Questions.Category.Criteria["education"], "tuition statements") {
			t.Fatalf("education criteria = %q", payload.Questions.Category.Criteria["education"])
		}
		if !strings.Contains(payload.Questions.Category.Criteria["receipts"], "no more specific domain") {
			t.Fatalf("receipts criteria = %q", payload.Questions.Category.Criteria["receipts"])
		}
		body := `{"answers":{"category":{"choice":"education","probabilities":{"education":0.9},"confidence":0.9},"sensitivity":{"score":1},"urgency":{"score":2},"needs_action":{"noul":0.8},"action_state":{"choice":"current","confidence":0.9}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}

	classification, err := (Jev{APIKey: "secret", BaseURL: "https://example.test", Client: client}).Classify(context.Background(), "Tuition statement")
	if err != nil {
		t.Fatal(err)
	}
	if classification.Category != "education" {
		t.Fatalf("category = %q", classification.Category)
	}
}

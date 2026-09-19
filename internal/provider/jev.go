package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/glamboyosa/docket/internal/domain"
)

type Classifier interface {
	Classify(context.Context, string) (domain.Classification, error)
}

type Jev struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

type jevRequest struct {
	State     jevState     `json:"state"`
	Model     string       `json:"model"`
	Questions jevQuestions `json:"questions"`
}

type jevState struct {
	Document string `json:"document"`
}

type jevQuestions struct {
	Category    choiceQuestion `json:"category"`
	Sensitivity scoreQuestion  `json:"sensitivity"`
	Urgency     scoreQuestion  `json:"urgency"`
	NeedsAction noulQuestion   `json:"needs_action"`
}

type choiceQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type scoreQuestion struct {
	Type         string   `json:"type"`
	Instructions string   `json:"instructions"`
	Criteria     []string `json:"criteria"`
}

type noulQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

func (j Jev) Classify(ctx context.Context, text string) (domain.Classification, error) {
	if j.BaseURL == "" {
		j.BaseURL = "https://api.typesafe.ai/v1/systemone"
	}
	if j.Client == nil {
		j.Client = &http.Client{Timeout: time.Minute}
	}
	payload := jevRequest{
		Model: "jev-latest",
		State: jevState{Document: text},
		Questions: jevQuestions{
			Category: choiceQuestion{
				Type:         "choice",
				Instructions: "Which single category best describes the primary domain and purpose of `document`? Prefer a domain-specific category over a generic financial record, invoice, or bill. Use other only when no specific category clearly applies.",
				Criteria: map[string]string{
					"tax": "Tax returns, tax forms, assessments, or tax authority correspondence", "legal": "Contracts, court papers, notices, or legal agreements",
					"financial": "Banking, investments, loans, account statements, or general financial records with no more specific domain", "medical": "Healthcare, prescriptions, test results, or medical records",
					"identity": "Identity, immigration, citizenship, or civil status documents", "insurance": "Insurance policies, claims, or coverage documents",
					"employment": "Employment contracts, pay records, reviews, or workplace documents", "education": "School records, tuition statements, certificates, transcripts, or course documents",
					"housing": "Leases, property, utilities, or housing documents", "receipts": "Proofs of purchase, paid receipts, or generic vendor invoices and bills with no more specific domain",
					"correspondence": "General letters, messages, or formal correspondence", "other": "None of the other categories clearly apply",
				},
			},
			Sensitivity: scoreQuestion{Type: "score", Instructions: "How sensitive is the information in `document`?", Criteria: []string{
				"Public or routine", "Personal", "Confidential", "Highly sensitive identity, health, legal, or financial data",
			}},
			Urgency: scoreQuestion{Type: "score", Instructions: "How urgently must the recipient respond to `document`?", Criteria: []string{
				"No deadline or action", "Action eventually", "Time-sensitive", "Immediate deadline, penalty, or serious consequence",
			}},
			NeedsAction: noulQuestion{Type: "noul", Instructions: "Does `document` ask or require the recipient to take an action?", Criteria: map[string]string{
				"true":  "The recipient is asked or required to respond, pay, submit, sign, attend, or complete another action",
				"false": "The document is informational or archival and requires no recipient action",
			}},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.Classification{}, fmt.Errorf("encode Jev request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.BaseURL, bytes.NewReader(body))
	if err != nil {
		return domain.Classification{}, fmt.Errorf("create Jev request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+j.APIKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := j.Client.Do(req)
	if err != nil {
		return domain.Classification{}, fmt.Errorf("send Jev request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return domain.Classification{}, fmt.Errorf("Jev request failed (%d): %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	var result struct {
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return domain.Classification{}, fmt.Errorf("decode Jev response: %w", err)
	}
	classification, err := parseJevAnswers(result.Answers)
	if err != nil {
		return domain.Classification{}, err
	}
	classification.Review = classification.CategoryConfidence < 0.55
	return classification, nil
}

func parseJevAnswers(answers map[string]json.RawMessage) (domain.Classification, error) {
	var category struct {
		Choice        string             `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
		Confidence    float64            `json:"confidence"`
	}
	var sensitivity, urgency struct {
		Score      float64 `json:"score"`
		Confidence float64 `json:"confidence"`
	}
	var action struct {
		Noul float64 `json:"noul"`
	}
	if err := json.Unmarshal(answers["category"], &category); err != nil || category.Choice == "" {
		return domain.Classification{}, fmt.Errorf("Jev returned no document category")
	}
	if err := json.Unmarshal(answers["sensitivity"], &sensitivity); err != nil {
		return domain.Classification{}, fmt.Errorf("decode Jev sensitivity: %w", err)
	}
	if err := json.Unmarshal(answers["urgency"], &urgency); err != nil {
		return domain.Classification{}, fmt.Errorf("decode Jev urgency: %w", err)
	}
	if err := json.Unmarshal(answers["needs_action"], &action); err != nil {
		return domain.Classification{}, fmt.Errorf("decode Jev action: %w", err)
	}
	return domain.Classification{
		Category: category.Choice, CategoryConfidence: category.Confidence, CategoryProbabilities: category.Probabilities,
		Sensitivity: sensitivity.Score, SensitivityConfidence: sensitivity.Confidence,
		Urgency: urgency.Score, UrgencyConfidence: urgency.Confidence,
		NeedsAction: action.Noul >= 0.65, NeedsActionProbability: action.Noul,
	}, nil
}

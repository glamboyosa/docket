package domain

import "time"

type Status string

const (
	StatusQueued      Status = "queued"
	StatusExtracting  Status = "extracting"
	StatusClassifying Status = "classifying"
	StatusFiled       Status = "filed"
	StatusFailed      Status = "failed"
)

var Categories = []string{
	"tax", "legal", "financial", "medical", "identity", "insurance",
	"employment", "education", "housing", "receipts", "correspondence", "other",
}

type Classification struct {
	Category               string             `json:"category"`
	CategoryConfidence     float64            `json:"category_confidence"`
	CategoryProbabilities  map[string]float64 `json:"category_probabilities"`
	Sensitivity            float64            `json:"sensitivity"`
	SensitivityConfidence  float64            `json:"sensitivity_confidence"`
	Urgency                float64            `json:"urgency"`
	UrgencyConfidence      float64            `json:"urgency_confidence"`
	NeedsAction            bool               `json:"needs_action"`
	NeedsActionProbability float64            `json:"needs_action_probability"`
	Review                 bool               `json:"review"`
	ActionAssessment       string             `json:"action_assessment"`
	ActionConfidence       float64            `json:"action_confidence"`
	ActionReview           bool               `json:"action_review"`
}

type Document struct {
	ID                     int64
	SHA256                 string
	OriginalName           string
	SourcePath             string
	LibraryPath            string
	Status                 Status
	Category               string
	CategoryConfidence     float64
	CategoryProbabilities  map[string]float64
	Sensitivity            float64
	Urgency                float64
	NeedsAction            bool
	NeedsActionProbability float64
	Review                 bool
	ActionAssessment       string
	ActionConfidence       float64
	ActionReview           bool
	ActionResolution       string
	Provider               string
	Model                  string
	Error                  string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

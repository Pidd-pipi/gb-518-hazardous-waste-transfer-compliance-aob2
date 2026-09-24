package dto

import "time"

// CreateComplianceCheck is the public write contract for 合规核验. Status is deliberately
// omitted so callers cannot bypass the service state machine.
type CreateComplianceCheck struct {
	Code          string    `json:"code" binding:"required,min=2,max=64"`
	Name          string    `json:"name" binding:"required,min=2,max=160"`
	ManifestCode  string    `json:"manifestCode" binding:"required,min=2,max=64"`
	Checklist     string    `json:"checklist" binding:"required,min=5,max=2000"`
	DecisionBasis string    `json:"decisionBasis" binding:"max=1000"`
	Description   string    `json:"description" binding:"max=1000"`
	Facility      string    `json:"facility" binding:"required,max=120"`
	Owner         string    `json:"owner" binding:"required,max=120"`
	Category      string    `json:"category" binding:"required,max=80"`
	RiskLevel     string    `json:"riskLevel" binding:"required,oneof=low medium high critical"`
	MetricValue   float64   `json:"metricValue"`
	MetricUnit    string    `json:"metricUnit" binding:"max=24"`
	EffectiveAt   time.Time `json:"effectiveAt" binding:"required"`
	Evidence      string    `json:"evidence" binding:"max=2000"`
	RelatedCode   string    `json:"relatedCode" binding:"max=64"`
}

type UpdateComplianceCheck struct {
	ExpectedVersion uint      `json:"expectedVersion" binding:"required"`
	Name            string    `json:"name" binding:"required,min=2,max=160"`
	ManifestCode    string    `json:"manifestCode" binding:"required,min=2,max=64"`
	Checklist       string    `json:"checklist" binding:"required,min=5,max=2000"`
	DecisionBasis   string    `json:"decisionBasis" binding:"max=1000"`
	Description     string    `json:"description" binding:"max=1000"`
	Facility        string    `json:"facility" binding:"required,max=120"`
	Owner           string    `json:"owner" binding:"required,max=120"`
	Category        string    `json:"category" binding:"required,max=80"`
	RiskLevel       string    `json:"riskLevel" binding:"required,oneof=low medium high critical"`
	MetricValue     float64   `json:"metricValue"`
	MetricUnit      string    `json:"metricUnit" binding:"max=24"`
	EffectiveAt     time.Time `json:"effectiveAt" binding:"required"`
	Evidence        string    `json:"evidence" binding:"max=2000"`
	RelatedCode     string    `json:"relatedCode" binding:"max=64"`
}

type DefectInput struct {
	Description string `json:"description" binding:"required,min=5,max=1000"`
	Category    string `json:"category" binding:"max=80"`
	Evidence    string `json:"evidence" binding:"max=1000"`
}

type DecideComplianceCheck struct {
	Status          string        `json:"status" binding:"required,oneof=pass fail escalated"`
	ExpectedVersion uint          `json:"expectedVersion" binding:"required"`
	Reason          string        `json:"reason" binding:"required,min=3,max=1000"`
	Assignee        string        `json:"assignee" binding:"max=120"`
	DueAt           *time.Time    `json:"dueAt"`
	Defects         []DefectInput `json:"defects" binding:"dive"`
}

type RemediationItemInput struct {
	RoundItemID  uint     `json:"roundItemId" binding:"required"`
	Note         string   `json:"note" binding:"required,min=3,max=1000"`
	EvidenceURLs []string `json:"evidenceUrls" binding:"required,min=1,dive,required,min=3,max=500"`
}

type SubmitRemediationRequest struct {
	ExpectedVersion uint                   `json:"expectedVersion" binding:"required"`
	RoundVersion    uint                   `json:"roundVersion" binding:"required"`
	Items           []RemediationItemInput `json:"items" binding:"required,min=1,dive"`
}

type ReviewRemediationItemRequest struct {
	RoundItemID uint   `json:"roundItemId" binding:"required"`
	Approved    bool   `json:"approved"`
	Comment     string `json:"comment" binding:"max=1000"`
}

type ReviewRemediationRequest struct {
	Action          string                         `json:"action" binding:"required,oneof=approve return"`
	ExpectedVersion uint                           `json:"expectedVersion" binding:"required"`
	RoundVersion    uint                           `json:"roundVersion" binding:"required"`
	Reason          string                         `json:"reason" binding:"required,min=3,max=1000"`
	Items           []ReviewRemediationItemRequest `json:"items" binding:"required,min=1,dive"`
}

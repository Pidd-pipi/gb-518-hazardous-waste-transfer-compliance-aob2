package dto

import "time"

// StartRectificationRequest extends a fail decision with the remediation plan:
// responsible person, deadline and one entry per failed checklist clause.
type StartRectificationRequest struct {
	ExpectedVersion uint                     `json:"expectedVersion" binding:"required"`
	Reason          string                   `json:"reason" binding:"required,min=3,max=500"`
	Assignee        string                   `json:"assignee" binding:"required,min=2,max=120"`
	DueAt           time.Time                `json:"dueAt" binding:"required"`
	Items           []RectificationItemInput `json:"items" binding:"required,min=1,dive"`
}

type RectificationItemInput struct {
	Clause      string `json:"clause" binding:"required,min=2,max=120"`
	Description string `json:"description" binding:"required,min=3,max=1000"`
	RiskLevel   string `json:"riskLevel" binding:"omitempty,oneof=low medium high critical"`
}

// SubmitRectificationRequest is the handler's response for one round. A note
// and evidence reference are required for every open defect so the reviewer can
// match the material back to the originating clause.
type SubmitRectificationRequest struct {
	ExpectedVersion uint                         `json:"expectedVersion" binding:"required"`
	Note            string                       `json:"note" binding:"required,min=3,max=2000"`
	Evidence        string                       `json:"evidence" binding:"max=2000"`
	Responses       []RectificationResponseInput `json:"responses" binding:"required,min=1,dive"`
}

type RectificationResponseInput struct {
	ItemSeq  uint   `json:"itemSeq" binding:"required"`
	Note     string `json:"note" binding:"required,min=2,max=1000"`
	Evidence string `json:"evidence" binding:"required,min=2,max=1000"`
}

// RecheckRequest is the reviewer's per-defect verdict. Every open defect must
// be marked explicitly; a single rejection returns the whole round with the
// accumulated reasons and the check goes back to rectifying.
type RecheckRequest struct {
	ExpectedVersion uint                        `json:"expectedVersion" binding:"required"`
	Verdicts        []RectificationVerdictInput `json:"verdicts" binding:"required,min=1,dive"`
}

type RectificationVerdictInput struct {
	ItemSeq  uint   `json:"itemSeq" binding:"required"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason" binding:"max=1000"`
}

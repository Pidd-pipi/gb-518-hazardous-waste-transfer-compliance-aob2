package model

import (
	"time"

	"gorm.io/gorm"
)

// Rectification is the online remediation aggregate opened when a 合规核验 is
// judged 不合格. The aggregate is append-only: every submission round and every
// per-defect response is retained on its own row; later rounds never overwrite
// earlier evidence.
type Rectification struct {
	BaseModel
	CheckID      uint       `json:"checkId" gorm:"uniqueIndex;not null"`
	CheckCode    string     `json:"checkCode" gorm:"size:64;index;not null"`
	ManifestCode string     `json:"manifestCode" gorm:"size:64;index;not null"`
	Assignee     string     `json:"assignee" gorm:"size:120;index;not null"`
	DueAt        time.Time  `json:"dueAt" gorm:"index;not null"`
	CurrentRound uint       `json:"currentRound" gorm:"not null;default:0"`
	ResolvedAt   *time.Time `json:"resolvedAt"`
}

func (item *Rectification) GetBase() *BaseModel { return &item.BaseModel }

func (Rectification) TableName() string { return "rectifications" }

var RectificationInitialStatus = "rectifying"

// RectificationStatus mirrors the check lifecycle while the aggregate is open:
// rectifying (办理人整改) -> recheck_pending (待复检) -> pass (闭环) or back to
// rectifying when the reviewer returns the round.
const (
	RectificationStatusRectifying     = "rectifying"
	RectificationStatusRecheckPending = "recheck_pending"
	RectificationStatusPass           = "pass"
)

// RectificationItem is one defect line captured at failure time. Each defect
// keeps its own sequence so supplementary material can be matched back to the
// exact clause it addresses.
type RectificationItem struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	RectificationID uint           `json:"rectificationId" gorm:"uniqueIndex:uniq_rect_item_seq;not null"`
	Seq             uint           `json:"seq" gorm:"uniqueIndex:uniq_rect_item_seq;not null"`
	Clause          string         `json:"clause" gorm:"size:120;not null"`
	Description     string         `json:"description" gorm:"size:1000;not null"`
	RiskLevel       string         `json:"riskLevel" gorm:"size:32;not null;default:medium"`
	ResolvedRound   uint           `json:"resolvedRound" gorm:"not null;default:0"`
	LatestResponse  string         `json:"latestResponse" gorm:"size:1000"`
	LatestEvidence  string         `json:"latestEvidence" gorm:"size:1000"`
	LatestRound     uint           `json:"latestRound" gorm:"not null;default:0"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

func (RectificationItem) TableName() string { return "rectification_items" }

// RectificationRound is an immutable, append-only row. Submitted rows carry the
// handler explanation and evidence for one material round; returned/approved
// rows carry the reviewer decision for that material round. RoundNo is a
// globally increasing sequence; SubmissionNo is the 1-based material round that
// the row belongs to.
type RectificationRound struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	RectificationID uint           `json:"rectificationId" gorm:"uniqueIndex:uniq_rect_round_no;not null"`
	RoundNo         uint           `json:"roundNo" gorm:"uniqueIndex:uniq_rect_round_no;not null"`
	SubmissionNo    uint           `json:"submissionNo" gorm:"index;not null"`
	Kind            string         `json:"kind" gorm:"size:32;not null"` // submitted | returned | approved
	Submitter       string         `json:"submitter" gorm:"size:120"`
	Note            string         `json:"note" gorm:"size:2000"`
	Evidence        string         `json:"evidence" gorm:"size:2000"`
	Reviewer        string         `json:"reviewer" gorm:"size:120"`
	ReviewNote      string         `json:"reviewNote" gorm:"size:2000"`
	AcceptedItems   int            `json:"acceptedItems" gorm:"not null;default:0"`
	RejectedItems   int            `json:"rejectedItems" gorm:"not null;default:0"`
	CreatedAt       time.Time      `json:"createdAt"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

func (RectificationRound) TableName() string { return "rectification_rounds" }

const (
	RoundKindSubmitted = "submitted"
	RoundKindReturned  = "returned"
	RoundKindApproved  = "approved"
)

// RectificationResponse is the per-defect answer inside one submitted material
// round. Rows are never edited in place: a re-submission creates a new round
// with a new set of responses, so every version of the supplementary material
// stays intact. Accepted/ReviewNote are filled once during review.
type RectificationResponse struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	RoundID         uint           `json:"roundId" gorm:"uniqueIndex:uniq_round_response;not null"`
	RectificationID uint           `json:"rectificationId" gorm:"index;not null"`
	SubmissionNo    uint           `json:"submissionNo" gorm:"index;not null"`
	ItemID          uint           `json:"itemId" gorm:"uniqueIndex:uniq_round_response;not null"`
	ItemSeq         uint           `json:"itemSeq" gorm:"not null"`
	Note            string         `json:"note" gorm:"size:1000;not null"`
	Evidence        string         `json:"evidence" gorm:"size:1000;not null"`
	Accepted        *bool          `json:"accepted"`
	ReviewNote      string         `json:"reviewNote" gorm:"size:1000"`
	CreatedAt       time.Time      `json:"createdAt"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

func (RectificationResponse) TableName() string { return "rectification_responses" }

package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	RemediationRoundStatusRectifying          = "rectifying"
	RemediationRoundStatusPendingReinspection = "pending_reinspection"
	RemediationRoundStatusReturned            = "returned"
	RemediationRoundStatusPassed              = "passed"

	RemediationItemStatusPending   = "pending"
	RemediationItemStatusSubmitted = "submitted"
	RemediationItemStatusApproved  = "approved"
	RemediationItemStatusRejected  = "rejected"
)

// RemediationDefect is the stable identity of one defect identified when a
// compliance check fails. Round-scoped snapshots retain the wording seen by
// the assignee during each remediation round.
type RemediationDefect struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	CheckID     uint           `json:"checkId" gorm:"index;not null;uniqueIndex:idx_remediation_defect_check_no,priority:1"`
	DefectNo    int            `json:"defectNo" gorm:"not null;uniqueIndex:idx_remediation_defect_check_no,priority:2"`
	Description string         `json:"description" gorm:"size:1000;not null"`
	Category    string         `json:"category" gorm:"size:80"`
	Evidence    string         `json:"evidence" gorm:"size:1000"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (RemediationDefect) TableName() string { return "remediation_defects" }

type RemediationRound struct {
	ID           uint                    `json:"id" gorm:"primaryKey"`
	CheckID      uint                    `json:"checkId" gorm:"index;not null;uniqueIndex:idx_remediation_round_check_no,priority:1"`
	RoundNo      int                     `json:"roundNo" gorm:"not null;uniqueIndex:idx_remediation_round_check_no,priority:2"`
	Assignee     string                  `json:"assignee" gorm:"size:120;index;not null"`
	DueAt        time.Time               `json:"dueAt" gorm:"index;not null"`
	Status       string                  `json:"status" gorm:"size:40;index;not null"`
	Version      uint                    `json:"version" gorm:"not null;default:1"`
	ReturnReason string                  `json:"returnReason" gorm:"size:1000"`
	SubmittedAt  *time.Time              `json:"submittedAt"`
	ReviewedBy   string                  `json:"reviewedBy" gorm:"size:80"`
	ReviewedAt   *time.Time              `json:"reviewedAt"`
	CreatedAt    time.Time               `json:"createdAt"`
	UpdatedAt    time.Time               `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt          `json:"-" gorm:"index"`
	Items        []RemediationRoundItem  `json:"items,omitempty" gorm:"foreignKey:RoundID"`
	Submissions  []RemediationSubmission `json:"submissions,omitempty" gorm:"foreignKey:RoundID"`
}

func (RemediationRound) TableName() string { return "remediation_rounds" }

type RemediationRoundItem struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	RoundID          uint           `json:"roundId" gorm:"index;not null;uniqueIndex:idx_remediation_round_item,priority:1"`
	DefectID         uint           `json:"defectId" gorm:"index;not null;uniqueIndex:idx_remediation_round_item,priority:2"`
	ItemNo           int            `json:"itemNo" gorm:"not null"`
	Description      string         `json:"description" gorm:"size:1000;not null"`
	Category         string         `json:"category" gorm:"size:80"`
	OriginalEvidence string         `json:"originalEvidence" gorm:"size:1000"`
	Status           string         `json:"status" gorm:"size:40;index;not null"`
	ReviewComment    string         `json:"reviewComment" gorm:"size:1000"`
	Version          uint           `json:"version" gorm:"not null;default:1"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

func (RemediationRoundItem) TableName() string { return "remediation_round_items" }

type RemediationSubmission struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CheckID      uint      `json:"checkId" gorm:"index;not null"`
	RoundID      uint      `json:"roundId" gorm:"index;not null"`
	RoundItemID  uint      `json:"roundItemId" gorm:"index;not null;uniqueIndex:idx_remediation_submission_item"`
	Note         string    `json:"note" gorm:"size:1000;not null"`
	EvidenceURLs []string  `json:"evidenceUrls" gorm:"serializer:json;type:text;not null"`
	SubmittedBy  string    `json:"submittedBy" gorm:"size:80;index;not null"`
	SubmittedAt  time.Time `json:"submittedAt" gorm:"index;not null"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (RemediationSubmission) TableName() string { return "remediation_submissions" }

type RemediationSummary struct {
	CurrentRoundNo int       `json:"currentRoundNo"`
	RoundStatus    string    `json:"roundStatus"`
	Assignee       string    `json:"assignee"`
	DueAt          time.Time `json:"dueAt"`
	TotalItems     int       `json:"totalItems"`
	SubmittedItems int       `json:"submittedItems"`
	ApprovedItems  int       `json:"approvedItems"`
	RejectedItems  int       `json:"rejectedItems"`
	LatestNote     string    `json:"latestNote"`
	LatestAt       time.Time `json:"latestAt"`
}

type RemediationDetail struct {
	Defects []RemediationDefect `json:"defects"`
	Rounds  []RemediationRound  `json:"rounds"`
}

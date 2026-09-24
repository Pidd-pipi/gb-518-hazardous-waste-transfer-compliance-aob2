package repository

import (
	"context"
	"strings"

	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/model"
	"gorm.io/gorm"
)

// ResponseVerdict carries one reviewer decision into the review transaction.
type ResponseVerdict struct {
	ResponseID uint
	Accepted   bool
	ReviewNote string
}

// ItemResolution carries the per-defect rollup update into the review or
// submit transaction (latest response snapshot and resolved marker).
type ItemResolution struct {
	ItemID         uint
	Resolved       bool
	LatestNote     string
	LatestEvidence string
	LatestRound    uint
}

// RectificationRepository owns the append-only remediation aggregate: one
// header per failed check, immutable defect items, immutable rounds and
// per-round per-defect responses.
type RectificationRepository interface {
	GetByCheckID(ctx context.Context, checkID uint) (model.Rectification, error)
	Items(ctx context.Context, rectificationID uint) ([]model.RectificationItem, error)
	Rounds(ctx context.Context, rectificationID uint) ([]model.RectificationRound, error)
	Responses(ctx context.Context, roundID uint) ([]model.RectificationResponse, error)
	ListByCheckIDs(ctx context.Context, checkIDs []uint) (map[uint]model.Rectification, error)
	CountRounds(ctx context.Context, rectificationID uint) (uint, error)
	// Open creates the header and defects, moves the check to rectifying and
	// writes the audit row in one transaction using the check optimistic lock.
	Open(ctx context.Context, header *model.Rectification, items []model.RectificationItem, checkID, expectedVersion uint, checkUpdate map[string]any, audit *model.AuditLog) error
	// SubmitRound appends a submitted round and responses, flips the check to
	// recheck_pending and refreshes defect rollups, all in one locked tx.
	SubmitRound(ctx context.Context, header *model.Rectification, round *model.RectificationRound, responses []model.RectificationResponse, itemRollups []ItemResolution, expectedVersion uint, checkUpdate map[string]any, audit *model.AuditLog) error
	// ReviewRound appends the decision round, records verdicts on responses and
	// items, moves the check to pass or rectifying, all in one locked tx.
	ReviewRound(ctx context.Context, header *model.Rectification, decisionRound *model.RectificationRound, verdicts []ResponseVerdict, itemRollups []ItemResolution, expectedVersion uint, checkUpdate map[string]any, audit *model.AuditLog) error
}

type rectificationRepository struct {
	db *gorm.DB
}

func NewRectificationRepository(db *gorm.DB) RectificationRepository {
	return &rectificationRepository{db: db}
}

func (r *rectificationRepository) GetByCheckID(ctx context.Context, checkID uint) (model.Rectification, error) {
	var header model.Rectification
	err := r.db.WithContext(ctx).Where("check_id = ?", checkID).First(&header).Error
	return header, err
}

func (r *rectificationRepository) Items(ctx context.Context, rectificationID uint) ([]model.RectificationItem, error) {
	var items []model.RectificationItem
	err := r.db.WithContext(ctx).Where("rectification_id = ?", rectificationID).Order("seq ASC").Find(&items).Error
	return items, err
}

func (r *rectificationRepository) Rounds(ctx context.Context, rectificationID uint) ([]model.RectificationRound, error) {
	var rounds []model.RectificationRound
	err := r.db.WithContext(ctx).Where("rectification_id = ?", rectificationID).Order("round_no ASC").Find(&rounds).Error
	return rounds, err
}

func (r *rectificationRepository) Responses(ctx context.Context, roundID uint) ([]model.RectificationResponse, error) {
	var responses []model.RectificationResponse
	err := r.db.WithContext(ctx).Where("round_id = ?", roundID).Order("item_seq ASC").Find(&responses).Error
	return responses, err
}

func (r *rectificationRepository) CountRounds(ctx context.Context, rectificationID uint) (uint, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.RectificationRound{}).
		Where("rectification_id = ?", rectificationID).Count(&count).Error; err != nil {
		return 0, err
	}
	return uint(count), nil
}

func (r *rectificationRepository) ListByCheckIDs(ctx context.Context, checkIDs []uint) (map[uint]model.Rectification, error) {
	result := make(map[uint]model.Rectification)
	if len(checkIDs) == 0 {
		return result, nil
	}
	var headers []model.Rectification
	if err := r.db.WithContext(ctx).Where("check_id IN ?", checkIDs).Find(&headers).Error; err != nil {
		return nil, err
	}
	for _, header := range headers {
		result[header.CheckID] = header
	}
	return result, nil
}

// isDuplicateKey reports whether a driver error is a unique-constraint
// violation across SQLite (UNIQUE constraint failed), PostgreSQL
// (23505 / duplicate key) and MySQL (1062 / Duplicate entry).
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "23505") ||
		strings.Contains(message, "duplicate entry") ||
		strings.Contains(message, "1062")
}

// lockCheck applies the optimistic-lock predicate and reports ErrVersionConflict
// when the caller's expected version is stale.
func lockCheck(tx *gorm.DB, checkID, expectedVersion uint, update map[string]any) error {
	result := tx.Model(&model.ComplianceCheck{}).
		Where("id = ? AND version = ?", checkID, expectedVersion).
		Updates(update)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *rectificationRepository) Open(ctx context.Context, header *model.Rectification, items []model.RectificationItem, checkID, expectedVersion uint, checkUpdate map[string]any, audit *model.AuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCheck(tx, checkID, expectedVersion, checkUpdate); err != nil {
			return err
		}
		if err := tx.Create(header).Error; err != nil {
			// Two concurrent open requests racing past the service-level guard
			// must surface as a conflict, never a 500 or a duplicate aggregate.
			if isDuplicateKey(err) {
				return ErrVersionConflict
			}
			return err
		}
		for index := range items {
			items[index].RectificationID = header.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		audit.EntityID = checkID
		return tx.Create(audit).Error
	})
}

func (r *rectificationRepository) SubmitRound(ctx context.Context, header *model.Rectification, round *model.RectificationRound, responses []model.RectificationResponse, itemRollups []ItemResolution, expectedVersion uint, checkUpdate map[string]any, audit *model.AuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCheck(tx, header.CheckID, expectedVersion, checkUpdate); err != nil {
			return err
		}
		if err := tx.Create(round).Error; err != nil {
			return err
		}
		for index := range responses {
			responses[index].RoundID = round.ID
			responses[index].RectificationID = header.ID
		}
		if err := tx.Create(&responses).Error; err != nil {
			return err
		}
		for _, rollup := range itemRollups {
			if err := tx.Model(&model.RectificationItem{}).Where("id = ?", rollup.ItemID).
				Updates(map[string]any{
					"latest_response": rollup.LatestNote,
					"latest_evidence": rollup.LatestEvidence,
					"latest_round":    rollup.LatestRound,
				}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.Rectification{}).Where("id = ?", header.ID).
			Updates(map[string]any{
				"status":        checkUpdate["status"],
				"version":       checkUpdate["version"],
				"current_round": round.SubmissionNo,
				"updated_at":    checkUpdate["updated_at"],
			}).Error; err != nil {
			return err
		}
		audit.EntityID = header.CheckID
		return tx.Create(audit).Error
	})
}

func (r *rectificationRepository) ReviewRound(ctx context.Context, header *model.Rectification, decisionRound *model.RectificationRound, verdicts []ResponseVerdict, itemRollups []ItemResolution, expectedVersion uint, checkUpdate map[string]any, audit *model.AuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCheck(tx, header.CheckID, expectedVersion, checkUpdate); err != nil {
			return err
		}
		if err := tx.Create(decisionRound).Error; err != nil {
			return err
		}
		for _, verdict := range verdicts {
			if err := tx.Model(&model.RectificationResponse{}).Where("id = ?", verdict.ResponseID).
				Updates(map[string]any{
					"accepted":    verdict.Accepted,
					"review_note": verdict.ReviewNote,
				}).Error; err != nil {
				return err
			}
		}
		for _, rollup := range itemRollups {
			updates := map[string]any{
				"latest_response": rollup.LatestNote,
				"latest_evidence": rollup.LatestEvidence,
				"latest_round":    rollup.LatestRound,
			}
			if rollup.Resolved {
				updates["resolved_round"] = rollup.LatestRound
			}
			if err := tx.Model(&model.RectificationItem{}).Where("id = ?", rollup.ItemID).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		headerUpdates := map[string]any{
			"status":     checkUpdate["status"],
			"version":    checkUpdate["version"],
			"updated_at": checkUpdate["updated_at"],
		}
		if checkUpdate["status"] == model.RectificationStatusPass {
			headerUpdates["resolved_at"] = checkUpdate["updated_at"]
		}
		if err := tx.Model(&model.Rectification{}).Where("id = ?", header.ID).
			Updates(headerUpdates).Error; err != nil {
			return err
		}
		audit.EntityID = header.CheckID
		return tx.Create(audit).Error
	})
}

package repository

import (
	"context"
	"time"

	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/dto"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/model"
	"gorm.io/gorm"
)

// ComplianceCheckRepository owns all persistence operations for 合规核验 and its
// immutable remediation rounds.
type ComplianceCheckRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.ComplianceCheck], error)
	Get(context.Context, uint) (model.ComplianceCheck, error)
	Create(context.Context, *model.ComplianceCheck) error
	CreateAudited(context.Context, *model.ComplianceCheck, *model.AuditLog) error
	Update(context.Context, uint, uint, *model.ComplianceCheck) error
	UpdateAudited(context.Context, uint, uint, *model.ComplianceCheck, *model.AuditLog) error
	CreateRemediationRoundAudited(context.Context, *model.ComplianceCheck, *model.RemediationRound, []model.RemediationDefect, *model.AuditLog) error
	SubmitRemediationAudited(context.Context, uint, uint, uint, uint, []dto.RemediationItemInput, string, *model.AuditLog) (model.ComplianceCheck, error)
	ReviewRemediationAudited(context.Context, uint, uint, uint, uint, dto.ReviewRemediationRequest, string, *model.AuditLog) (model.ComplianceCheck, error)
	GetRemediationDetail(context.Context, uint) (model.RemediationDetail, error)
	Delete(context.Context, uint) error
	DeleteAudited(context.Context, uint, *model.AuditLog) error
	CountByStatus(context.Context) (map[string]int64, error)
}

type complianceCheckRepository struct {
	store *Store[model.ComplianceCheck]
	db    *gorm.DB
}

func NewComplianceCheckRepository(db *gorm.DB) ComplianceCheckRepository {
	return &complianceCheckRepository{store: NewStore[model.ComplianceCheck](db), db: db}
}

func (r *complianceCheckRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.ComplianceCheck], error) {
	page, err := r.store.List(ctx, q)
	if err != nil {
		return page, err
	}
	if err := r.attachRemediation(ctx, page.Items); err != nil {
		return Page[model.ComplianceCheck]{}, err
	}
	return page, nil
}

func (r *complianceCheckRepository) Get(ctx context.Context, id uint) (model.ComplianceCheck, error) {
	item, err := r.store.Get(ctx, id)
	if err != nil {
		return item, err
	}
	items := []model.ComplianceCheck{item}
	if err := r.attachRemediation(ctx, items); err != nil {
		return model.ComplianceCheck{}, err
	}
	return items[0], nil
}

func (r *complianceCheckRepository) Create(ctx context.Context, item *model.ComplianceCheck) error {
	return r.store.Create(ctx, item)
}
func (r *complianceCheckRepository) CreateAudited(ctx context.Context, item *model.ComplianceCheck, audit *model.AuditLog) error {
	return r.store.CreateAudited(ctx, item, audit)
}
func (r *complianceCheckRepository) Update(ctx context.Context, id, version uint, item *model.ComplianceCheck) error {
	return r.store.Update(ctx, id, version, item)
}
func (r *complianceCheckRepository) UpdateAudited(ctx context.Context, id, version uint, item *model.ComplianceCheck, audit *model.AuditLog) error {
	return r.store.UpdateAudited(ctx, id, version, item, audit)
}
func (r *complianceCheckRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *complianceCheckRepository) DeleteAudited(ctx context.Context, id uint, audit *model.AuditLog) error {
	return r.store.DeleteAudited(ctx, id, audit)
}
func (r *complianceCheckRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

func (r *complianceCheckRepository) CreateRemediationRoundAudited(ctx context.Context, check *model.ComplianceCheck, round *model.RemediationRound, defects []model.RemediationDefect, audit *model.AuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		result := tx.Model(&model.ComplianceCheck{}).
			Where("id = ? AND version = ?", check.ID, check.Version).
			Updates(map[string]any{"status": check.Status, "version": check.Version + 1, "updated_at": now, "decision_basis": check.DecisionBasis})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		for index := range defects {
			defects[index].CheckID = check.ID
			if err := tx.Create(&defects[index]).Error; err != nil {
				return err
			}
		}
		round.CheckID = check.ID
		if err := tx.Create(round).Error; err != nil {
			return err
		}
		for index, defect := range defects {
			item := model.RemediationRoundItem{
				RoundID: round.ID, DefectID: defect.ID, ItemNo: index + 1,
				Description: defect.Description, Category: defect.Category, OriginalEvidence: defect.Evidence,
				Status: model.RemediationItemStatusPending, Version: 1, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		check.Version++
		check.UpdatedAt = now
		audit.EntityID = check.ID
		return tx.Create(audit).Error
	})
}

func (r *complianceCheckRepository) SubmitRemediationAudited(ctx context.Context, checkID, expectedVersion, roundID, roundVersion uint, inputs []dto.RemediationItemInput, actor string, audit *model.AuditLog) (model.ComplianceCheck, error) {
	var check model.ComplianceCheck
	var round model.RemediationRound
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&check, checkID).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ? AND check_id = ?", roundID, checkID).First(&round).Error; err != nil {
			return err
		}
		if check.Version != expectedVersion || round.Version != roundVersion {
			return ErrVersionConflict
		}
		if round.Status != model.RemediationRoundStatusRectifying {
			return gorm.ErrInvalidData
		}
		now := time.Now().UTC()
		itemByID := make(map[uint]model.RemediationRoundItem)
		var items []model.RemediationRoundItem
		if err := tx.Where("round_id = ?", roundID).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			itemByID[item.ID] = item
		}
		seen := make(map[uint]bool)
		for _, input := range inputs {
			if seen[input.RoundItemID] {
				return gorm.ErrDuplicatedKey
			}
			seen[input.RoundItemID] = true
			item, exists := itemByID[input.RoundItemID]
			if !exists || item.Status != model.RemediationItemStatusPending {
				return gorm.ErrInvalidData
			}
			submission := model.RemediationSubmission{
				CheckID: checkID, RoundID: roundID, RoundItemID: item.ID,
				Note: input.Note, EvidenceURLs: input.EvidenceURLs,
				SubmittedBy: actor, SubmittedAt: now, CreatedAt: now,
			}
			if err := tx.Create(&submission).Error; err != nil {
				return err
			}
			result := tx.Model(&model.RemediationRoundItem{}).
				Where("id = ? AND version = ?", item.ID, item.Version).
				Updates(map[string]any{"status": model.RemediationItemStatusSubmitted, "version": item.Version + 1, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrVersionConflict
			}
		}
		if len(seen) != len(items) {
			return gorm.ErrInvalidData
		}
		result := tx.Model(&model.RemediationRound{}).
			Where("id = ? AND version = ?", roundID, roundVersion).
			Updates(map[string]any{"status": model.RemediationRoundStatusPendingReinspection, "version": roundVersion + 1, "updated_at": now, "submitted_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		result = tx.Model(&model.ComplianceCheck{}).
			Where("id = ? AND version = ?", checkID, expectedVersion).
			Updates(map[string]any{"status": "pending_reinspection", "version": expectedVersion + 1, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		audit.EntityID = checkID
		return tx.Create(audit).Error
	})
	if err != nil {
		return model.ComplianceCheck{}, mapPersistenceError(err)
	}
	return r.Get(ctx, checkID)
}

func (r *complianceCheckRepository) ReviewRemediationAudited(ctx context.Context, checkID, expectedVersion, roundID, roundVersion uint, input dto.ReviewRemediationRequest, actor string, audit *model.AuditLog) (model.ComplianceCheck, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var (
			check model.ComplianceCheck
			round model.RemediationRound
		)
		if err := tx.First(&check, checkID).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ? AND check_id = ?", roundID, checkID).First(&round).Error; err != nil {
			return err
		}
		if check.Version != expectedVersion || round.Version != roundVersion {
			return ErrVersionConflict
		}
		if round.Status != model.RemediationRoundStatusPendingReinspection {
			return gorm.ErrInvalidData
		}
		now := time.Now().UTC()
		var items []model.RemediationRoundItem
		if err := tx.Where("round_id = ?", roundID).Order("item_no ASC").Find(&items).Error; err != nil {
			return err
		}
		itemByID := make(map[uint]model.RemediationRoundItem)
		for _, item := range items {
			itemByID[item.ID] = item
		}
		decisions := make(map[uint]dto.ReviewRemediationItemRequest)
		rejected := 0
		for _, decision := range input.Items {
			if _, exists := itemByID[decision.RoundItemID]; !exists {
				return gorm.ErrInvalidData
			}
			if _, duplicate := decisions[decision.RoundItemID]; duplicate {
				return gorm.ErrDuplicatedKey
			}
			decisions[decision.RoundItemID] = decision
			if !decision.Approved {
				rejected++
			}
		}
		if len(decisions) != len(items) {
			return gorm.ErrInvalidData
		}
		nextRoundNo := round.RoundNo + 1
		var nextRound *model.RemediationRound
		if input.Action == "return" {
			nextRound = &model.RemediationRound{
				CheckID: checkID, RoundNo: nextRoundNo, Assignee: round.Assignee, DueAt: round.DueAt,
				Status: model.RemediationRoundStatusRectifying, Version: 1, ReturnReason: input.Reason,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(nextRound).Error; err != nil {
				return err
			}
		}
		for _, item := range items {
			decision := decisions[item.ID]
			status := model.RemediationItemStatusApproved
			if !decision.Approved {
				status = model.RemediationItemStatusRejected
			}
			result := tx.Model(&model.RemediationRoundItem{}).
				Where("id = ? AND version = ?", item.ID, item.Version).
				Updates(map[string]any{"status": status, "review_comment": decision.Comment, "version": item.Version + 1, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrVersionConflict
			}
			if !decision.Approved {
				nextItem := model.RemediationRoundItem{
					RoundID: nextRound.ID, DefectID: item.DefectID, ItemNo: item.ItemNo,
					Description: item.Description, Category: item.Category, OriginalEvidence: item.OriginalEvidence,
					Status: model.RemediationItemStatusPending, Version: 1, CreatedAt: now, UpdatedAt: now,
				}
				if err := tx.Create(&nextItem).Error; err != nil {
					return err
				}
			}
		}
		roundStatus := model.RemediationRoundStatusPassed
		checkStatus := "pass"
		if input.Action == "return" {
			roundStatus = model.RemediationRoundStatusReturned
			checkStatus = "fail"
		}
		reviewedAt := now
		result := tx.Model(&model.RemediationRound{}).
			Where("id = ? AND version = ?", roundID, roundVersion).
			Updates(map[string]any{"status": roundStatus, "version": roundVersion + 1, "updated_at": now, "return_reason": input.Reason, "reviewed_by": actor, "reviewed_at": reviewedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		result = tx.Model(&model.ComplianceCheck{}).
			Where("id = ? AND version = ?", checkID, expectedVersion).
			Updates(map[string]any{"status": checkStatus, "version": expectedVersion + 1, "updated_at": now, "decision_basis": input.Reason})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		audit.EntityID = checkID
		return tx.Create(audit).Error
	})
	if err != nil {
		return model.ComplianceCheck{}, mapPersistenceError(err)
	}
	return r.Get(ctx, checkID)
}

func (r *complianceCheckRepository) GetRemediationDetail(ctx context.Context, checkID uint) (model.RemediationDetail, error) {
	var defects []model.RemediationDefect
	if err := r.db.WithContext(ctx).Where("check_id = ?", checkID).Order("defect_no ASC").Find(&defects).Error; err != nil {
		return model.RemediationDetail{}, err
	}
	var rounds []model.RemediationRound
	if err := r.db.WithContext(ctx).Where("check_id = ?", checkID).Order("round_no ASC").Find(&rounds).Error; err != nil {
		return model.RemediationDetail{}, err
	}
	if len(rounds) > 0 {
		roundIDs := make([]uint, 0, len(rounds))
		for _, round := range rounds {
			roundIDs = append(roundIDs, round.ID)
		}
		var items []model.RemediationRoundItem
		if err := r.db.WithContext(ctx).Where("round_id IN ?", roundIDs).Order("item_no ASC").Find(&items).Error; err != nil {
			return model.RemediationDetail{}, err
		}
		var submissions []model.RemediationSubmission
		if err := r.db.WithContext(ctx).Where("round_id IN ?", roundIDs).Order("submitted_at ASC, id ASC").Find(&submissions).Error; err != nil {
			return model.RemediationDetail{}, err
		}
		itemsByRound := make(map[uint][]model.RemediationRoundItem)
		submissionsByRound := make(map[uint][]model.RemediationSubmission)
		for _, item := range items {
			itemsByRound[item.RoundID] = append(itemsByRound[item.RoundID], item)
		}
		for _, submission := range submissions {
			submissionsByRound[submission.RoundID] = append(submissionsByRound[submission.RoundID], submission)
		}
		for index := range rounds {
			rounds[index].Items = itemsByRound[rounds[index].ID]
			rounds[index].Submissions = submissionsByRound[rounds[index].ID]
		}
	}
	return model.RemediationDetail{Defects: defects, Rounds: rounds}, nil
}

func (r *complianceCheckRepository) attachRemediation(ctx context.Context, checks []model.ComplianceCheck) error {
	if len(checks) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(checks))
	for _, check := range checks {
		ids = append(ids, check.ID)
	}
	var rounds []model.RemediationRound
	if err := r.db.WithContext(ctx).Where("check_id IN ?", ids).Order("round_no ASC").Find(&rounds).Error; err != nil {
		return err
	}
	latestRound := make(map[uint]model.RemediationRound)
	for _, round := range rounds {
		latestRound[round.CheckID] = round
	}
	roundIDs := make([]uint, 0, len(rounds))
	for _, round := range rounds {
		roundIDs = append(roundIDs, round.ID)
	}
	summaryByCheck := make(map[uint]model.RemediationSummary)
	if len(roundIDs) > 0 {
		var items []model.RemediationRoundItem
		if err := r.db.WithContext(ctx).Where("round_id IN ?", roundIDs).Find(&items).Error; err != nil {
			return err
		}
		roundByID := make(map[uint]model.RemediationRound, len(rounds))
		for _, round := range rounds {
			roundByID[round.ID] = round
		}
		latestItemKey := make(map[checkDefectKey]model.RemediationRoundItem)
		for _, item := range items {
			round := roundByID[item.RoundID]
			key := checkDefectKey{checkID: round.CheckID, defectID: item.DefectID}
			current, exists := latestItemKey[key]
			currentRound := roundByID[current.RoundID]
			if !exists || round.RoundNo > currentRound.RoundNo {
				latestItemKey[key] = item
			}
		}
		for _, item := range latestItemKey {
			round := roundByID[item.RoundID]
			summary := summaryByCheck[round.CheckID]
			summary.TotalItems++
			switch item.Status {
			case model.RemediationItemStatusSubmitted:
				summary.SubmittedItems++
			case model.RemediationItemStatusApproved:
				summary.ApprovedItems++
			case model.RemediationItemStatusRejected:
				summary.RejectedItems++
			}
			summaryByCheck[round.CheckID] = summary
		}
		var submissions []model.RemediationSubmission
		if err := r.db.WithContext(ctx).Where("round_id IN ?", roundIDs).Order("submitted_at ASC, id ASC").Find(&submissions).Error; err != nil {
			return err
		}
		latestSubmission := make(map[uint]model.RemediationSubmission)
		for _, submission := range submissions {
			if latest, hasLatest := latestSubmission[submission.CheckID]; !hasLatest || submission.SubmittedAt.After(latest.SubmittedAt) {
				latestSubmission[submission.CheckID] = submission
			}
		}
		for index := range checks {
			round, exists := latestRound[checks[index].ID]
			if !exists {
				continue
			}
			summary := summaryByCheck[checks[index].ID]
			summary.CurrentRoundNo = round.RoundNo
			summary.RoundStatus = round.Status
			summary.Assignee = round.Assignee
			summary.DueAt = round.DueAt
			if submission, exists := latestSubmission[checks[index].ID]; exists {
				summary.LatestNote = submission.Note
				summary.LatestAt = submission.SubmittedAt
			}
			checks[index].Remediation = &summary
			summaryByCheck[checks[index].ID] = summary
		}
	}
	return nil
}

type checkDefectKey struct {
	checkID  uint
	defectID uint
}

func mapPersistenceError(err error) error {
	switch err {
	case gorm.ErrInvalidData:
		return ErrInvalidState
	case gorm.ErrDuplicatedKey:
		return ErrDuplicateAction
	default:
		return err
	}
}

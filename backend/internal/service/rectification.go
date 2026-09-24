package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/constants"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/dto"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/model"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/repository"
)

// DefectView is a single failed clause with its newest per-defect response.
type DefectView struct {
	Seq            uint   `json:"seq"`
	Clause         string `json:"clause"`
	Description    string `json:"description"`
	RiskLevel      string `json:"riskLevel"`
	Resolved       bool   `json:"resolved"`
	ResolvedRound  uint   `json:"resolvedRound"`
	LatestRound    uint   `json:"latestRound"`
	LatestResponse string `json:"latestResponse"`
	LatestEvidence string `json:"latestEvidence"`
}

// ResponseView is one per-defect answer kept inside an immutable round.
type ResponseView struct {
	ItemSeq    uint   `json:"itemSeq"`
	Note       string `json:"note"`
	Evidence   string `json:"evidence"`
	Accepted   *bool  `json:"accepted"`
	ReviewNote string `json:"reviewNote"`
}

// RoundView is one retained material round or reviewer decision.
type RoundView struct {
	RoundNo       uint           `json:"roundNo"`
	SubmissionNo  uint           `json:"submissionNo"`
	Kind          string         `json:"kind"`
	Submitter     string         `json:"submitter"`
	Note          string         `json:"note"`
	Evidence      string         `json:"evidence"`
	Reviewer      string         `json:"reviewer"`
	ReviewNote    string         `json:"reviewNote"`
	AcceptedItems int            `json:"acceptedItems"`
	RejectedItems int            `json:"rejectedItems"`
	CreatedAt     time.Time      `json:"createdAt"`
	Responses     []ResponseView `json:"responses"`
}

// RectificationView is the aggregate rendered in the remediation drawer.
type RectificationView struct {
	CheckID        uint         `json:"checkId"`
	CheckCode      string       `json:"checkCode"`
	CheckStatus    string       `json:"checkStatus"`
	Version        uint         `json:"version"`
	ManifestCode   string       `json:"manifestCode"`
	Assignee       string       `json:"assignee"`
	DueAt          time.Time    `json:"dueAt"`
	CurrentRound   uint         `json:"currentRound"`
	ResolvedAt     *time.Time   `json:"resolvedAt"`
	Items          []DefectView `json:"items"`
	Rounds         []RoundView  `json:"rounds"`
	TotalItems     int          `json:"totalItems"`
	ResolvedItems  int          `json:"resolvedItems"`
	LatestRoundNo  uint         `json:"latestRoundNo"`
	LatestNote     string       `json:"latestNote"`
	LatestEvidence string       `json:"latestEvidence"`
	CreatedAt      time.Time    `json:"createdAt"`
}

// RectificationSummary is the compact progress row embedded in the check list.
type RectificationSummary struct {
	CheckID        uint      `json:"checkId"`
	Status         string    `json:"status"`
	Assignee       string    `json:"assignee"`
	DueAt          time.Time `json:"dueAt"`
	CurrentRound   uint      `json:"currentRound"`
	TotalItems     int       `json:"totalItems"`
	ResolvedItems  int       `json:"resolvedItems"`
	LatestRoundNo  uint      `json:"latestRoundNo"`
	LatestNote     string    `json:"latestNote"`
	LatestEvidence string    `json:"latestEvidence"`
}

type RectificationService interface {
	Open(context.Context, uint, dto.StartRectificationRequest, string, string) (RectificationView, error)
	Submit(context.Context, uint, dto.SubmitRectificationRequest, string, string) (RectificationView, error)
	Review(context.Context, uint, dto.RecheckRequest, string, string) (RectificationView, error)
	GetByCheckID(context.Context, uint) (RectificationView, error)
	Summaries(context.Context, []uint) (map[uint]RectificationSummary, error)
}

type rectificationService struct {
	checks repository.ComplianceCheckRepository
	repo   repository.RectificationRepository
}

func NewRectificationService(checks repository.ComplianceCheckRepository, repo repository.RectificationRepository) RectificationService {
	return &rectificationService{checks: checks, repo: repo}
}

func (s *rectificationService) Open(ctx context.Context, checkID uint, input dto.StartRectificationRequest, actor, requestID string) (RectificationView, error) {
	check, err := s.checks.Get(ctx, checkID)
	if err != nil {
		return RectificationView{}, err
	}
	// Guard against opening a second aggregate for the same check before any
	// version comparison: an existing remediation is the strongest conflict
	// signal and must surface as 409 even when the client is stale.
	if existing, err := s.repo.GetByCheckID(ctx, checkID); err == nil && existing.ID != 0 {
		return RectificationView{}, fmt.Errorf("%w: remediation already exists for this check and cannot be overwritten", ErrConflict)
	}
	if check.Status != string(constants.CheckStateFail) {
		return RectificationView{}, fmt.Errorf("%w: remediation can only start from a failed check", ErrConflict)
	}
	dueAt := input.DueAt.UTC()
	if !dueAt.After(time.Now().UTC()) {
		return RectificationView{}, fmt.Errorf("%w: remediation deadline must be in the future", ErrInvalidInput)
	}
	items := make([]model.RectificationItem, 0, len(input.Items))
	seen := make(map[uint]bool, len(input.Items))
	for index, entry := range input.Items {
		seq := uint(index + 1)
		risk := strings.TrimSpace(entry.RiskLevel)
		if risk == "" {
			risk = "medium"
		}
		if seen[seq] {
			return RectificationView{}, fmt.Errorf("%w: defect sequence must be unique", ErrInvalidInput)
		}
		seen[seq] = true
		items = append(items, model.RectificationItem{
			Seq: seq, Clause: strings.TrimSpace(entry.Clause),
			Description: strings.TrimSpace(entry.Description), RiskLevel: risk,
		})
	}
	now := time.Now().UTC()
	header := model.Rectification{
		BaseModel: model.BaseModel{
			Code:        "RC-" + check.Code,
			Name:        "整改-" + check.Name,
			Status:      model.RectificationStatusRectifying,
			Version:     input.ExpectedVersion + 1,
			Description: "核验不合格后发起的线上整改复检",
		},
		CheckID:      check.ID,
		CheckCode:    check.Code,
		ManifestCode: check.ManifestCode,
		Assignee:     strings.TrimSpace(input.Assignee),
		DueAt:        dueAt,
	}
	checkUpdate := map[string]any{
		"status":         model.RectificationStatusRectifying,
		"version":        input.ExpectedVersion + 1,
		"updated_at":     now,
		"decision_basis": strings.TrimSpace(input.Reason),
	}
	audit := newAuditLog(actor, requestID, "rectify_open", "ComplianceCheck", check.Status, model.RectificationStatusRectifying,
		fmt.Sprintf("opened remediation assignee=%s due=%s defects=%d reason=%s", header.Assignee, dueAt.Format(time.RFC3339), len(items), strings.TrimSpace(input.Reason)))
	if err := s.repo.Open(ctx, &header, items, check.ID, input.ExpectedVersion, checkUpdate, audit); err != nil {
		return RectificationView{}, err
	}
	return s.GetByCheckID(ctx, check.ID)
}

func (s *rectificationService) Submit(ctx context.Context, checkID uint, input dto.SubmitRectificationRequest, actor, requestID string) (RectificationView, error) {
	header, err := s.repo.GetByCheckID(ctx, checkID)
	if err != nil {
		return RectificationView{}, err
	}
	if header.Status != model.RectificationStatusRectifying {
		return RectificationView{}, fmt.Errorf("%w: material can only be submitted while remediation is open", ErrConflict)
	}
	if header.Version != input.ExpectedVersion {
		return RectificationView{}, fmt.Errorf("%w: check version is stale, refresh before submitting", repository.ErrVersionConflict)
	}
	items, err := s.repo.Items(ctx, header.ID)
	if err != nil {
		return RectificationView{}, err
	}
	openItems := make(map[uint]model.RectificationItem)
	for _, item := range items {
		if item.ResolvedRound == 0 {
			openItems[item.Seq] = item
		}
	}
	if len(openItems) == 0 {
		return RectificationView{}, fmt.Errorf("%w: all defects are already resolved", ErrConflict)
	}
	if len(input.Responses) != len(openItems) {
		return RectificationView{}, fmt.Errorf("%w: every open defect needs one remediation response (%d required, %d provided)", ErrInvalidInput, len(openItems), len(input.Responses))
	}
	roundCount, err := s.repo.CountRounds(ctx, header.ID)
	if err != nil {
		return RectificationView{}, err
	}
	submissionNo := header.CurrentRound + 1
	round := model.RectificationRound{
		RectificationID: header.ID,
		RoundNo:         roundCount + 1,
		SubmissionNo:    submissionNo,
		Kind:            model.RoundKindSubmitted,
		Submitter:       actor,
		Note:            strings.TrimSpace(input.Note),
		Evidence:        strings.TrimSpace(input.Evidence),
	}
	responses := make([]model.RectificationResponse, 0, len(input.Responses))
	rollups := make([]repository.ItemResolution, 0, len(input.Responses))
	used := make(map[uint]bool, len(input.Responses))
	for _, entry := range input.Responses {
		item, ok := openItems[entry.ItemSeq]
		if !ok {
			return RectificationView{}, fmt.Errorf("%w: response for defect %d does not match an open defect", ErrInvalidInput, entry.ItemSeq)
		}
		if used[entry.ItemSeq] {
			return RectificationView{}, fmt.Errorf("%w: defect %d has duplicated responses in one round", ErrInvalidInput, entry.ItemSeq)
		}
		used[entry.ItemSeq] = true
		responses = append(responses, model.RectificationResponse{
			RectificationID: header.ID,
			SubmissionNo:    submissionNo,
			ItemID:          item.ID,
			ItemSeq:         item.Seq,
			Note:            strings.TrimSpace(entry.Note),
			Evidence:        strings.TrimSpace(entry.Evidence),
		})
		rollups = append(rollups, repository.ItemResolution{
			ItemID: item.ID, LatestNote: strings.TrimSpace(entry.Note),
			LatestEvidence: strings.TrimSpace(entry.Evidence), LatestRound: submissionNo,
		})
	}
	now := time.Now().UTC()
	checkUpdate := map[string]any{
		"status":     model.RectificationStatusRecheckPending,
		"version":    header.Version + 1,
		"updated_at": now,
	}
	audit := newAuditLog(actor, requestID, "rectify_submit", "ComplianceCheck", model.RectificationStatusRectifying, model.RectificationStatusRecheckPending,
		fmt.Sprintf("submitted remediation round %d with %d defect responses", submissionNo, len(responses)))
	if err := s.repo.SubmitRound(ctx, &header, &round, responses, rollups, input.ExpectedVersion, checkUpdate, audit); err != nil {
		return RectificationView{}, err
	}
	return s.GetByCheckID(ctx, checkID)
}

func (s *rectificationService) Review(ctx context.Context, checkID uint, input dto.RecheckRequest, actor, requestID string) (RectificationView, error) {
	header, err := s.repo.GetByCheckID(ctx, checkID)
	if err != nil {
		return RectificationView{}, err
	}
	if header.Status != model.RectificationStatusRecheckPending {
		return RectificationView{}, fmt.Errorf("%w: only a pending recheck can be reviewed", ErrConflict)
	}
	if header.Version != input.ExpectedVersion {
		return RectificationView{}, fmt.Errorf("%w: check version is stale, refresh before reviewing", repository.ErrVersionConflict)
	}
	rounds, err := s.repo.Rounds(ctx, header.ID)
	if err != nil {
		return RectificationView{}, err
	}
	var latest *model.RectificationRound
	for i := range rounds {
		if rounds[i].Kind == model.RoundKindSubmitted && rounds[i].SubmissionNo == header.CurrentRound {
			latest = &rounds[i]
		}
	}
	if latest == nil {
		return RectificationView{}, fmt.Errorf("%w: submitted material for round %d is missing", ErrConflict, header.CurrentRound)
	}
	responses, err := s.repo.Responses(ctx, latest.ID)
	if err != nil {
		return RectificationView{}, err
	}
	responseBySeq := make(map[uint]model.RectificationResponse, len(responses))
	for _, response := range responses {
		responseBySeq[response.ItemSeq] = response
	}
	verdicts := make([]repository.ResponseVerdict, 0, len(input.Verdicts))
	verdictBySeq := make(map[uint]dto.RectificationVerdictInput, len(input.Verdicts))
	for _, verdict := range input.Verdicts {
		if _, exists := verdictBySeq[verdict.ItemSeq]; exists {
			return RectificationView{}, fmt.Errorf("%w: defect %d is scored more than once", ErrInvalidInput, verdict.ItemSeq)
		}
		response, ok := responseBySeq[verdict.ItemSeq]
		if !ok {
			return RectificationView{}, fmt.Errorf("%w: verdict for defect %d does not match the submitted round", ErrInvalidInput, verdict.ItemSeq)
		}
		if !verdict.Accepted && strings.TrimSpace(verdict.Reason) == "" {
			return RectificationView{}, fmt.Errorf("%w: rejecting defect %d requires a reason", ErrInvalidInput, verdict.ItemSeq)
		}
		verdictBySeq[verdict.ItemSeq] = verdict
		verdicts = append(verdicts, repository.ResponseVerdict{
			ResponseID: response.ID, Accepted: verdict.Accepted, ReviewNote: strings.TrimSpace(verdict.Reason),
		})
	}
	if len(verdictBySeq) != len(responseBySeq) {
		return RectificationView{}, fmt.Errorf("%w: every submitted defect must receive a verdict (%d required, %d provided)", ErrInvalidInput, len(responseBySeq), len(verdictBySeq))
	}
	allAccepted := true
	rejectedReasons := make([]string, 0)
	for seq, verdict := range verdictBySeq {
		if !verdict.Accepted {
			allAccepted = false
			rejectedReasons = append(rejectedReasons, fmt.Sprintf("#%d %s", seq, strings.TrimSpace(verdict.Reason)))
		}
	}

	targetStatus := model.RectificationStatusRectifying
	roundKind := model.RoundKindReturned
	auditAction := "rectify_return"
	if allAccepted {
		targetStatus = model.RectificationStatusPass
		roundKind = model.RoundKindApproved
		auditAction = "rectify_approve"
	}
	roundCount, err := s.repo.CountRounds(ctx, header.ID)
	if err != nil {
		return RectificationView{}, err
	}
	acceptedCount, rejectedCount := 0, 0
	for _, verdict := range verdictBySeq {
		if verdict.Accepted {
			acceptedCount++
		} else {
			rejectedCount++
		}
	}
	decisionRound := model.RectificationRound{
		RectificationID: header.ID,
		RoundNo:         roundCount + 1,
		SubmissionNo:    latest.SubmissionNo,
		Kind:            roundKind,
		Reviewer:        actor,
		ReviewNote:      strings.Join(rejectedReasons, "; "),
		AcceptedItems:   acceptedCount,
		RejectedItems:   rejectedCount,
	}
	// Roll the latest response snapshot onto each defect; accepted defects close.
	items, err := s.repo.Items(ctx, header.ID)
	if err != nil {
		return RectificationView{}, err
	}
	itemBySeq := make(map[uint]model.RectificationItem, len(items))
	for _, item := range items {
		itemBySeq[item.Seq] = item
	}
	rollups := make([]repository.ItemResolution, 0, len(verdicts))
	for _, response := range responses {
		verdict := verdictBySeq[response.ItemSeq]
		rollups = append(rollups, repository.ItemResolution{
			ItemID:         itemBySeq[response.ItemSeq].ID,
			Resolved:       verdict.Accepted,
			LatestNote:     response.Note,
			LatestEvidence: response.Evidence,
			LatestRound:    latest.SubmissionNo,
		})
	}
	now := time.Now().UTC()
	checkUpdate := map[string]any{
		"status":     targetStatus,
		"version":    header.Version + 1,
		"updated_at": now,
	}
	auditDetail := fmt.Sprintf("reviewed round %d accepted=%d rejected=%d", latest.SubmissionNo, acceptedCount, rejectedCount)
	if targetStatus == model.RectificationStatusRectifying {
		auditDetail = fmt.Sprintf("returned round %d: %s", latest.SubmissionNo, strings.Join(rejectedReasons, "; "))
	}
	audit := newAuditLog(actor, requestID, auditAction, "ComplianceCheck", model.RectificationStatusRecheckPending, targetStatus, auditDetail)
	if err := s.repo.ReviewRound(ctx, &header, &decisionRound, verdicts, rollups, input.ExpectedVersion, checkUpdate, audit); err != nil {
		return RectificationView{}, err
	}
	return s.GetByCheckID(ctx, checkID)
}

func (s *rectificationService) GetByCheckID(ctx context.Context, checkID uint) (RectificationView, error) {
	header, err := s.repo.GetByCheckID(ctx, checkID)
	if err != nil {
		return RectificationView{}, err
	}
	items, err := s.repo.Items(ctx, header.ID)
	if err != nil {
		return RectificationView{}, err
	}
	rounds, err := s.repo.Rounds(ctx, header.ID)
	if err != nil {
		return RectificationView{}, err
	}
	view := RectificationView{
		CheckID: header.CheckID, CheckCode: header.CheckCode, CheckStatus: header.Status,
		Version: header.Version, ManifestCode: header.ManifestCode, Assignee: header.Assignee,
		DueAt: header.DueAt, CurrentRound: header.CurrentRound, ResolvedAt: header.ResolvedAt,
		CreatedAt: header.CreatedAt,
	}
	for _, item := range items {
		view.Items = append(view.Items, DefectView{
			Seq: item.Seq, Clause: item.Clause, Description: item.Description, RiskLevel: item.RiskLevel,
			Resolved: item.ResolvedRound > 0, ResolvedRound: item.ResolvedRound, LatestRound: item.LatestRound,
			LatestResponse: item.LatestResponse, LatestEvidence: item.LatestEvidence,
		})
	}
	view.TotalItems = len(view.Items)
	for _, round := range rounds {
		responses, err := s.repo.Responses(ctx, round.ID)
		if err != nil {
			return RectificationView{}, err
		}
		rv := RoundView{
			RoundNo: round.RoundNo, SubmissionNo: round.SubmissionNo, Kind: round.Kind,
			Submitter: round.Submitter, Note: round.Note, Evidence: round.Evidence,
			Reviewer: round.Reviewer, ReviewNote: round.ReviewNote,
			AcceptedItems: round.AcceptedItems, RejectedItems: round.RejectedItems, CreatedAt: round.CreatedAt,
		}
		for _, response := range responses {
			rv.Responses = append(rv.Responses, ResponseView{
				ItemSeq: response.ItemSeq, Note: response.Note, Evidence: response.Evidence,
				Accepted: response.Accepted, ReviewNote: response.ReviewNote,
			})
		}
		view.Rounds = append(view.Rounds, rv)
	}
	for _, item := range view.Items {
		if item.Resolved {
			view.ResolvedItems++
		}
	}
	for i := len(view.Rounds) - 1; i >= 0; i-- {
		if view.Rounds[i].Kind == model.RoundKindSubmitted {
			view.LatestRoundNo = view.Rounds[i].SubmissionNo
			view.LatestNote = view.Rounds[i].Note
			view.LatestEvidence = view.Rounds[i].Evidence
			break
		}
	}
	return view, nil
}

func (s *rectificationService) Summaries(ctx context.Context, checkIDs []uint) (map[uint]RectificationSummary, error) {
	headers, err := s.repo.ListByCheckIDs(ctx, checkIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[uint]RectificationSummary, len(headers))
	for checkID, header := range headers {
		items, err := s.repo.Items(ctx, header.ID)
		if err != nil {
			return nil, err
		}
		summary := RectificationSummary{
			CheckID: checkID, Status: header.Status, Assignee: header.Assignee, DueAt: header.DueAt,
			CurrentRound: header.CurrentRound, TotalItems: len(items),
		}
		for _, item := range items {
			if item.ResolvedRound > 0 {
				summary.ResolvedItems++
			}
			if item.LatestRound >= summary.LatestRoundNo && item.LatestRound > 0 {
				summary.LatestRoundNo = item.LatestRound
				summary.LatestNote = item.LatestResponse
				summary.LatestEvidence = item.LatestEvidence
			}
		}
		result[checkID] = summary
	}
	return result, nil
}

package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/database"
	"github.com/blueship581/hazardous-waste-transfer-compliance/backend/internal/router"
	"github.com/gin-gonic/gin"
)

// TestRectificationWorkflow exercises the full online remediation loop:
// fail -> open with assignee/deadline/defects -> submit -> return -> resubmit
// -> approve, asserting append-only rounds, optimistic-lock conflicts and RBAC.
func TestRectificationWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testConfig(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, _, err := database.Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	engine := router.New(cfg, db, nil, logger)

	operator := login(t, engine, "operator")
	reviewer := login(t, engine, "reviewer")
	viewer := login(t, engine, "viewer")

	response, _ := request(t, engine, http.MethodPost, "/api/manifests", operator, "rect-manifest-create", manifestPayload("TM-RECT-001", "CP-002"))
	assertStatus(t, response, http.StatusCreated)
	manifest := decodeRecord(t, responseBody(t, response))
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/manifests/%d/transition", manifest.ID), operator, "rect-manifest-submit", map[string]any{
		"status": "submitted", "expectedVersion": manifest.Version, "reason": "linked permits checked",
	})
	assertStatus(t, response, http.StatusOK)

	response, body := request(t, engine, http.MethodPost, "/api/checks", operator, "rect-check-create", checkPayload("CC-RECT-001", manifest.Code))
	assertStatus(t, response, http.StatusCreated)
	check := decodeRecord(t, body)

	// Reviewer judges the check 不合格 first.
	response, body = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/transition", check.ID), reviewer, "rect-check-fail", map[string]any{
		"status": "fail", "expectedVersion": check.Version, "reason": "weight proof and destination permit gaps",
	})
	assertStatus(t, response, http.StatusOK)
	check = decodeRecord(t, body)
	if check.Status != "fail" || check.Version != 2 {
		t.Fatalf("expected fail v2, got %+v", check)
	}

	dueAt := time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339)
	openPayload := map[string]any{
		"expectedVersion": check.Version,
		"reason":          "重量凭证差异、处置去向凭证缺失，限期整改",
		"assignee":        "现场整改组-王敏",
		"dueAt":           dueAt,
		"items": []map[string]any{
			{"clause": "联单重量复核", "description": "过磅单重量与联单申报重量相差 320kg", "riskLevel": "high"},
			{"clause": "处置去向凭证", "description": "缺少接收单位盖章的处置回执", "riskLevel": "medium"},
		},
	}

	// RBAC: operator and viewer cannot open remediation.
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/open", check.ID), operator, "rect-open-forbidden", openPayload)
	assertStatus(t, response, http.StatusForbidden)
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/open", check.ID), viewer, "rect-open-forbidden-v", openPayload)
	assertStatus(t, response, http.StatusForbidden)

	response, body = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/open", check.ID), reviewer, "rect-open", openPayload)
	assertStatus(t, response, http.StatusCreated)
	rect := decodeRectification(t, body)
	if rect.CheckStatus != "rectifying" || rect.Version != 3 || rect.Assignee != "现场整改组-王敏" {
		t.Fatalf("unexpected opened remediation: %+v", rect)
	}
	if len(rect.Items) != 2 || rect.TotalItems != 2 || rect.Items[0].Seq != 1 || rect.Items[1].Seq != 2 {
		t.Fatalf("expected two ordered defects, got %+v", rect.Items)
	}

	// Duplicate open must conflict and never overwrite the first record.
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/open", check.ID), reviewer, "rect-open-dup", openPayload)
	assertStatus(t, response, http.StatusConflict)

	// Stale version on open also conflicts.
	stale := openPayload
	stale["expectedVersion"] = 1
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/open", check.ID), reviewer, "rect-open-stale", stale)
	assertStatus(t, response, http.StatusConflict)

	// Missing a per-defect response is rejected.
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/submit", check.ID), operator, "rect-submit-incomplete", map[string]any{
		"expectedVersion": rect.Version,
		"note":            "第一轮整改说明",
		"evidence":        "minio://evidence/rect/r1-pack.zip",
		"responses": []map[string]any{
			{"itemSeq": 1, "note": "已重新过磅并上传磅单", "evidence": "minio://evidence/rect/r1/weight.pdf"},
		},
	})
	assertStatus(t, response, http.StatusUnprocessableEntity)

	// Full first submission moves the check to 待复检.
	response, body = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/submit", check.ID), operator, "rect-submit-r1", map[string]any{
		"expectedVersion": rect.Version,
		"note":            "第一轮整改说明：两项缺陷材料已补齐",
		"evidence":        "minio://evidence/rect/r1-pack.zip",
		"responses": []map[string]any{
			{"itemSeq": 1, "note": "已重新过磅并上传磅单", "evidence": "minio://evidence/rect/r1/weight.pdf"},
			{"itemSeq": 2, "note": "已补盖接收单位公章", "evidence": "minio://evidence/rect/r1/receipt.pdf"},
		},
	})
	assertStatus(t, response, http.StatusOK)
	rect = decodeRectification(t, body)
	if rect.CheckStatus != "recheck_pending" || rect.CurrentRound != 1 || rect.Version != 4 {
		t.Fatalf("unexpected pending recheck state: %+v", rect)
	}

	// Duplicated / stale submission conflicts; the first round stays intact.
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/submit", check.ID), operator, "rect-submit-dup", map[string]any{
		"expectedVersion": 3,
		"note":            "重复提交必须冲突",
		"responses": []map[string]any{
			{"itemSeq": 1, "note": "旧版本不得覆盖", "evidence": "minio://evidence/stale-a"},
			{"itemSeq": 2, "note": "旧版本不得覆盖", "evidence": "minio://evidence/stale-b"},
		},
	})
	assertStatus(t, response, http.StatusConflict)

	// Reviewer cannot approve without scoring every defect.
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/review", check.ID), reviewer, "rect-review-partial", map[string]any{
		"expectedVersion": rect.Version,
		"verdicts": []map[string]any{
			{"itemSeq": 1, "accepted": true},
		},
	})
	assertStatus(t, response, http.StatusUnprocessableEntity)

	// Operator cannot recheck.
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/review", check.ID), operator, "rect-review-forbidden", map[string]any{
		"expectedVersion": rect.Version,
		"verdicts": []map[string]any{
			{"itemSeq": 1, "accepted": true},
			{"itemSeq": 2, "accepted": false, "reason": "回执印章仍不清晰"},
		},
	})
	assertStatus(t, response, http.StatusForbidden)

	// One rejection returns the whole round and the check goes back to rectifying.
	response, body = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/review", check.ID), reviewer, "rect-review-return", map[string]any{
		"expectedVersion": rect.Version,
		"verdicts": []map[string]any{
			{"itemSeq": 1, "accepted": true},
			{"itemSeq": 2, "accepted": false, "reason": "回执印章仍不清晰，请重新盖章"},
		},
	})
	assertStatus(t, response, http.StatusOK)
	rect = decodeRectification(t, body)
	if rect.CheckStatus != "rectifying" || rect.Version != 5 || rect.ResolvedItems != 1 {
		t.Fatalf("expected return with one resolved defect, got status=%s version=%d resolved=%d", rect.CheckStatus, rect.Version, rect.ResolvedItems)
	}
	if rect.Items[0].ResolvedRound != 1 || rect.Items[1].ResolvedRound != 0 {
		t.Fatalf("unexpected per-defect resolution: %+v", rect.Items)
	}

	// Stale review on the already-decided round conflicts.
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/review", check.ID), reviewer, "rect-review-stale", map[string]any{
		"expectedVersion": 4,
		"verdicts": []map[string]any{
			{"itemSeq": 1, "accepted": true},
			{"itemSeq": 2, "accepted": true},
		},
	})
	assertStatus(t, response, http.StatusConflict)

	// Second round only needs the still-open defect.
	response, body = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/submit", check.ID), operator, "rect-submit-r2", map[string]any{
		"expectedVersion": rect.Version,
		"note":            "第二轮整改：重新取得盖章回执",
		"evidence":        "minio://evidence/rect/r2-pack.zip",
		"responses": []map[string]any{
			{"itemSeq": 2, "note": "已重新取得处置单位盖章回执", "evidence": "minio://evidence/rect/r2/receipt.pdf"},
		},
	})
	assertStatus(t, response, http.StatusOK)
	rect = decodeRectification(t, body)
	if rect.CheckStatus != "recheck_pending" || rect.CurrentRound != 2 || rect.Version != 6 {
		t.Fatalf("unexpected second pending recheck: %+v", rect)
	}

	// All accepted closes the check as pass.
	response, body = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/review", check.ID), reviewer, "rect-review-approve", map[string]any{
		"expectedVersion": rect.Version,
		"verdicts": []map[string]any{
			{"itemSeq": 2, "accepted": true},
		},
	})
	assertStatus(t, response, http.StatusOK)
	rect = decodeRectification(t, body)
	if rect.CheckStatus != "pass" || rect.Version != 7 || rect.ResolvedItems != 2 || rect.ResolvedAt == nil {
		t.Fatalf("expected approved closed remediation, got %+v", rect)
	}
	if len(rect.Rounds) != 4 {
		t.Fatalf("every material round and decision must be retained, got %d rounds: %+v", len(rect.Rounds), rect.Rounds)
	}
	wantKinds := []string{"submitted", "returned", "submitted", "approved"}
	for i, round := range rect.Rounds {
		if round.Kind != wantKinds[i] {
			t.Fatalf("round %d kind = %s, want %s", i+1, round.Kind, wantKinds[i])
		}
	}
	if len(rect.Rounds[0].Responses) != 2 || len(rect.Rounds[2].Responses) != 1 {
		t.Fatalf("per-round responses must stay attached to their own round: %+v", rect.Rounds)
	}
	if rect.Rounds[1].ReviewNote == "" || rect.Rounds[1].RejectedItems != 1 || rect.Rounds[1].AcceptedItems != 1 {
		t.Fatalf("returned round must keep the reviewer reason and counts: %+v", rect.Rounds[1])
	}

	// Closed remediation rejects further submissions.
	response, _ = request(t, engine, http.MethodPost, fmt.Sprintf("/api/checks/%d/rectification/submit", check.ID), operator, "rect-submit-closed", map[string]any{
		"expectedVersion": rect.Version,
		"note":            "闭环后禁止再提交",
		"responses": []map[string]any{
			{"itemSeq": 1, "note": "闭环后不得再补材料", "evidence": "minio://evidence/closed"},
		},
	})
	assertStatus(t, response, http.StatusConflict)

	// The check itself is now pass, and its history carries every remediation action.
	response, body = request(t, engine, http.MethodGet, fmt.Sprintf("/api/checks/%d", check.ID), viewer, "rect-check-final", nil)
	assertStatus(t, response, http.StatusOK)
	finalCheck := decodeRecord(t, body)
	if finalCheck.Status != "pass" {
		t.Fatalf("check should be pass after remediation, got %+v", finalCheck)
	}
	response, body = request(t, engine, http.MethodGet, fmt.Sprintf("/api/audits/ComplianceCheck/%d?limit=50", check.ID), reviewer, "rect-audit", nil)
	assertStatus(t, response, http.StatusOK)
	for _, marker := range []string{"rectify_open", "rectify_submit", "rectify_return", "rectify_approve"} {
		if !bytes.Contains(body, []byte(marker)) {
			t.Fatalf("audit history missing %s: %s", marker, string(body))
		}
	}

	// List summaries expose progress, deadline and the latest note.
	response, body = request(t, engine, http.MethodGet, fmt.Sprintf("/api/checks/rectification-summaries?ids=%d", check.ID), viewer, "rect-summary", nil)
	assertStatus(t, response, http.StatusOK)
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode summaries: %v body=%s", err, string(body))
	}
	summary, ok := envelope.Data[fmt.Sprintf("%d", check.ID)]
	if !ok || !bytes.Contains(summary, []byte(`"status":"pass"`)) || !bytes.Contains(summary, []byte(`"resolvedItems":2`)) {
		t.Fatalf("summary missing progress data: %s", string(body))
	}
}

type rectificationRecord struct {
	CheckStatus   string  `json:"checkStatus"`
	Version       uint    `json:"version"`
	Assignee      string  `json:"assignee"`
	CurrentRound  uint    `json:"currentRound"`
	ResolvedItems int     `json:"resolvedItems"`
	TotalItems    int     `json:"totalItems"`
	ResolvedAt    *string `json:"resolvedAt"`
	Items         []struct {
		Seq           uint `json:"seq"`
		ResolvedRound uint `json:"resolvedRound"`
	} `json:"items"`
	Rounds []struct {
		Kind          string `json:"kind"`
		ReviewNote    string `json:"reviewNote"`
		AcceptedItems int    `json:"acceptedItems"`
		RejectedItems int    `json:"rejectedItems"`
		Responses     []struct {
			ItemSeq uint `json:"itemSeq"`
		} `json:"responses"`
	} `json:"rounds"`
}

func decodeRectification(t *testing.T, body []byte) rectificationRecord {
	t.Helper()
	var envelope struct {
		Data rectificationRecord `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Data.CheckStatus == "" {
		t.Fatalf("decode rectification: %v body=%s", err, string(body))
	}
	return envelope.Data
}

func responseBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return data
}

// Shared types for the 整改复检 (remediation / recheck) workflow. The shape
// mirrors the backend RectificationView so the drawer can render every retained
// material round without extra mapping.

export type RectificationStatus = 'rectifying' | 'recheck_pending' | 'pass';

export interface DefectItem {
  seq: number;
  clause: string;
  description: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  resolved: boolean;
  resolvedRound: number;
  latestRound: number;
  latestResponse: string;
  latestEvidence: string;
}

export interface DefectResponse {
  itemSeq: number;
  note: string;
  evidence: string;
  accepted: boolean | null;
  reviewNote: string;
}

export type RoundKind = 'submitted' | 'returned' | 'approved';

export interface DefectRound {
  roundNo: number;
  submissionNo: number;
  kind: RoundKind;
  submitter: string;
  note: string;
  evidence: string;
  reviewer: string;
  reviewNote: string;
  acceptedItems: number;
  rejectedItems: number;
  createdAt: string;
  responses: DefectResponse[];
}

export interface Rectification {
  checkId: number;
  checkCode: string;
  checkStatus: RectificationStatus;
  version: number;
  manifestCode: string;
  assignee: string;
  dueAt: string;
  currentRound: number;
  resolvedAt: string | null;
  items: DefectItem[];
  rounds: DefectRound[];
  totalItems: number;
  resolvedItems: number;
  latestRoundNo: number;
  latestNote: string;
  latestEvidence: string;
  createdAt: string;
}

export interface RectificationSummary {
  checkId: number;
  status: RectificationStatus;
  assignee: string;
  dueAt: string;
  currentRound: number;
  totalItems: number;
  resolvedItems: number;
  latestRoundNo: number;
  latestNote: string;
  latestEvidence: string;
}

export interface StartRectificationInput {
  expectedVersion: number;
  reason: string;
  assignee: string;
  dueAt: string;
  items: { clause: string; description: string; riskLevel: string }[];
}

export interface SubmitRectificationInput {
  expectedVersion: number;
  note: string;
  evidence: string;
  responses: { itemSeq: number; note: string; evidence: string }[];
}

export interface RecheckInput {
  expectedVersion: number;
  verdicts: { itemSeq: number; accepted: boolean; reason: string }[];
}

export const RECTIFICATION_STATUS_LABEL: Record<RectificationStatus, string> = {
  rectifying: '整改中',
  recheck_pending: '待复检',
  pass: '复检通过',
};

export const ROUND_KIND_LABEL: Record<RoundKind, string> = {
  submitted: '办理人提交',
  returned: '复核退回',
  approved: '复检通过',
};

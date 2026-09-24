import type { DomainRecord } from './domain';

export type CheckState = 'pending' | 'pass' | 'fail' | 'pending_reinspection' | 'escalated';
export const ALL_CHECK_STATE: readonly CheckState[] = ['pending', 'pass', 'fail', 'pending_reinspection', 'escalated'];

export interface DefectInput {
  description: string;
  category?: string;
  evidence?: string;
}

export interface ComplianceDecisionInput {
  status: 'pass' | 'fail' | 'escalated';
  expectedVersion: number;
  reason: string;
  assignee?: string;
  dueAt?: string;
  defects?: DefectInput[];
}

export interface RemediationItemInput {
  roundItemId: number;
  note: string;
  evidenceUrls: string[];
}

export interface SubmitRemediationInput {
  expectedVersion: number;
  roundVersion: number;
  items: RemediationItemInput[];
}

export interface ReviewItemInput {
  roundItemId: number;
  approved: boolean;
  comment?: string;
}

export interface ReviewRemediationInput {
  action: 'approve' | 'return';
  expectedVersion: number;
  roundVersion: number;
  reason: string;
  items: ReviewItemInput[];
}

export interface ComplianceCheck extends DomainRecord {
  remediation?: RemediationSummary | null;
}

export interface RemediationSummary {
  currentRoundNo: number;
  roundStatus: RoundStatus;
  assignee: string;
  dueAt: string;
  totalItems: number;
  submittedItems: number;
  approvedItems: number;
  rejectedItems: number;
  latestNote: string;
  latestAt: string;
}

export type RoundStatus = 'rectifying' | 'pending_reinspection' | 'returned' | 'passed';
export type RemediationItemStatus = 'pending' | 'submitted' | 'approved' | 'rejected';

export interface RemediationDefect {
  id: number;
  checkId: number;
  defectNo: number;
  description: string;
  category: string;
  evidence: string;
  createdAt: string;
  updatedAt: string;
}

export interface RemediationRoundItem {
  id: number;
  roundId: number;
  defectId: number;
  itemNo: number;
  description: string;
  category: string;
  originalEvidence: string;
  status: RemediationItemStatus;
  reviewComment: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface RemediationSubmission {
  id: number;
  checkId: number;
  roundId: number;
  roundItemId: number;
  note: string;
  evidenceUrls: string[];
  submittedBy: string;
  submittedAt: string;
  createdAt: string;
}

export interface RemediationRound {
  id: number;
  checkId: number;
  roundNo: number;
  assignee: string;
  dueAt: string;
  status: RoundStatus;
  version: number;
  returnReason: string;
  submittedAt?: string | null;
  reviewedBy: string;
  reviewedAt?: string | null;
  createdAt: string;
  updatedAt: string;
  items: RemediationRoundItem[];
  submissions: RemediationSubmission[];
}

export interface RemediationDetail {
  defects: RemediationDefect[];
  rounds: RemediationRound[];
}

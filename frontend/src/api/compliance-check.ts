import { request } from './client';
import type { DomainRecord } from '../types/domain';
import type { ComplianceCheck, ComplianceDecisionInput, RemediationDetail, ReviewRemediationInput, SubmitRemediationInput } from '../types/compliance';

export async function listComplianceCheck(page = 1, pageSize = 20, search = '') {
  return request<ComplianceCheck[]>(`/checks?page=${page}&pageSize=${pageSize}&search=${encodeURIComponent(search)}`);
}
export async function createComplianceCheck(input: Partial<DomainRecord>) {
  return request<ComplianceCheck>('/checks', { method: 'POST', body: JSON.stringify(input) });
}
export async function transitionComplianceCheck(id: number, input: ComplianceDecisionInput) {
  return request<ComplianceCheck>(`/checks/${id}/transition`, { method: 'POST', body: JSON.stringify(input) });
}
export async function getRemediation(id: number) {
  return request<RemediationDetail>(`/checks/${id}/remediation`);
}
export async function submitRemediation(checkId: number, roundId: number, input: SubmitRemediationInput) {
  return request<ComplianceCheck>(`/checks/${checkId}/remediation/rounds/${roundId}/submit`, { method: 'POST', body: JSON.stringify(input) });
}
export async function reviewRemediation(checkId: number, roundId: number, input: ReviewRemediationInput) {
  return request<ComplianceCheck>(`/checks/${checkId}/remediation/rounds/${roundId}/review`, { method: 'POST', body: JSON.stringify(input) });
}

import { request } from './client';
import type {
  RecheckInput,
  Rectification,
  RectificationSummary,
  StartRectificationInput,
  SubmitRectificationInput,
} from '../types/rectification';

export async function getRectification(checkId: number) {
  return request<Rectification>(`/checks/${checkId}/rectification`);
}

export async function listRectificationSummaries(checkIds: number[]) {
  if (!checkIds.length) return { data: {} as Record<string, RectificationSummary> };
  const query = checkIds.map((id) => `ids=${id}`).join('&');
  return request<Record<string, RectificationSummary>>(`/checks/rectification-summaries?${query}`);
}

export async function openRectification(checkId: number, input: StartRectificationInput) {
  return request<Rectification>(`/checks/${checkId}/rectification/open`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function submitRectification(checkId: number, input: SubmitRectificationInput) {
  return request<Rectification>(`/checks/${checkId}/rectification/submit`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function reviewRectification(checkId: number, input: RecheckInput) {
  return request<Rectification>(`/checks/${checkId}/rectification/review`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

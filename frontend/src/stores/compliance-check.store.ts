import { Injectable } from '@angular/core';
import { BehaviorSubject } from 'rxjs';
import { createComplianceCheck, getRemediation, listComplianceCheck, reviewRemediation, submitRemediation, transitionComplianceCheck } from '../api/compliance-check';
import type { DomainRecord, PageMeta } from '../types/domain';
import type { ComplianceCheck, ComplianceDecisionInput, RemediationDetail, ReviewRemediationInput, SubmitRemediationInput } from '../types/compliance';

export interface ComplianceState {
  items: ComplianceCheck[];
  meta: PageMeta;
  detail: RemediationDetail | null;
  loading: boolean;
  error: string;
}

const initialState: ComplianceState = {
  items: [],
  meta: { page: 1, pageSize: 10, total: 0 },
  detail: null,
  loading: false,
  error: '',
};

@Injectable({ providedIn: 'root' })
export class ComplianceCheckStore {
  private readonly subject = new BehaviorSubject<ComplianceState>(initialState);
  readonly state$ = this.subject.asObservable();

  get snapshot(): ComplianceState {
    return this.subject.value;
  }

  async load(search = '', page = 1, pageSize = 10): Promise<void> {
    this.patch({ loading: true, error: '' });
    try {
      const result = await listComplianceCheck(page, pageSize, search);
      this.subject.next({
        ...this.subject.value,
        items: result.data,
        meta: result.meta || { page, pageSize, total: result.data.length },
        loading: false,
        error: '',
      });
    } catch (error) {
      this.patch({ loading: false, error: this.message(error) });
    }
  }

  async create(input: Partial<DomainRecord>): Promise<void> {
    this.patch({ loading: true, error: '' });
    try {
      await createComplianceCheck(input);
      await this.load();
    } catch (error) {
      this.patch({ loading: false, error: this.message(error) });
      throw error;
    }
  }

  async decide(id: number, input: ComplianceDecisionInput): Promise<void> {
    this.patch({ loading: true, error: '' });
    try {
      await transitionComplianceCheck(id, input);
      await this.load();
      if (this.snapshot.detail) await this.loadRemediation(id);
    } catch (error) {
      this.patch({ loading: false, error: this.message(error) });
      throw error;
    }
  }

  async loadRemediation(id: number): Promise<void> {
    this.patch({ loading: true, error: '' });
    try {
      const result = await getRemediation(id);
      this.patch({ detail: result.data, loading: false, error: '' });
    } catch (error) {
      this.patch({ loading: false, error: this.message(error) });
    }
  }

  async submit(checkId: number, roundId: number, input: SubmitRemediationInput): Promise<void> {
    this.patch({ loading: true, error: '' });
    try {
      await submitRemediation(checkId, roundId, input);
      await this.load();
      await this.loadRemediation(checkId);
    } catch (error) {
      this.patch({ loading: false, error: this.message(error) });
      throw error;
    }
  }

  async review(checkId: number, roundId: number, input: ReviewRemediationInput): Promise<void> {
    this.patch({ loading: true, error: '' });
    try {
      await reviewRemediation(checkId, roundId, input);
      await this.load();
      await this.loadRemediation(checkId);
    } catch (error) {
      this.patch({ loading: false, error: this.message(error) });
      throw error;
    }
  }

  clearDetail(): void {
    this.patch({ detail: null });
  }

  private patch(value: Partial<ComplianceState>): void {
    this.subject.next({ ...this.subject.value, ...value });
  }

  private message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
}

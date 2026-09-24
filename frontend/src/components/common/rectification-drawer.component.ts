import { CommonModule } from '@angular/common';
import { ChangeDetectorRef, Component, EventEmitter, Input, OnChanges, Output } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { useAuth } from '../../hooks/use-auth';
import { openRectification, reviewRectification, submitRectification } from '../../api/rectification';
import type { DomainRecord } from '../../types/domain';
import {
  RECTIFICATION_STATUS_LABEL,
  ROUND_KIND_LABEL,
  type DefectItem,
  type Rectification,
} from '../../types/rectification';
import { formatDate } from '../../utils/format';

interface DefectDraft {
  clause: string;
  description: string;
  riskLevel: string;
}

interface ResponseDraft {
  itemSeq: number;
  note: string;
  evidence: string;
}

interface VerdictDraft {
  itemSeq: number;
  accepted: boolean;
  reason: string;
}

type Mode = 'open' | 'submit' | 'review' | null;

@Component({
  selector: 'app-rectification-drawer',
  standalone: true,
  imports: [CommonModule, FormsModule, MatButtonModule],
  template: `
    <div class="modal-backdrop" (click)="close.emit()">
      <section class="drawer" role="dialog" aria-modal="true" aria-labelledby="rect-title" (click)="$event.stopPropagation()">
        <header class="drawer-header">
          <div>
            <p class="eyebrow">整改复检</p>
            <h2 id="rect-title">{{ check.code }} · {{ check.name }}</h2>
            <small>联单 {{ check.manifestCode }} · 决定依据：{{ check.decisionBasis || '—' }}</small>
          </div>
          <button mat-button (click)="close.emit()">关闭</button>
        </header>

        <div *ngIf="loading" class="drawer-loading">正在载入整改记录…</div>
        <div *ngIf="error" class="alert" role="alert">{{ error }}</div>

        <ng-container *ngIf="!loading">
          <!-- 尚未发起整改：复核员在 fail 时填写责任人、期限与逐条缺陷 -->
          <div *ngIf="!data" class="drawer-body">
            <p class="hint" *ngIf="!canOpen()">该核验当前未发起整改，需由复核员在“不合格”状态下发起。</p>
            <div *ngIf="canOpen()" class="form-grid">
              <label>不合格原因 / 整改要求
                <textarea [(ngModel)]="openReason" rows="2" placeholder="说明判定不合格的依据与总体整改要求"></textarea>
              </label>
              <label>整改责任人
                <input [(ngModel)]="assignee" placeholder="如：现场整改组-王敏" />
              </label>
              <label>整改期限
                <input type="datetime-local" [(ngModel)]="dueAtLocal" />
              </label>
            </div>

            <div *ngIf="canOpen()" class="defect-editor">
              <div class="block-title"><strong>逐条缺陷</strong><button mat-button type="button" (click)="addDefect()">+ 添加缺陷</button></div>
              <article *ngFor="let defect of defectDrafts; let i = index" class="defect-card">
                <header><span>#{{ i + 1 }}</span><button mat-button type="button" (click)="removeDefect(i)">删除</button></header>
                <label>缺陷条款 / 对应检查项
                  <input [(ngModel)]="defect.clause" placeholder="如：联单重量复核" />
                </label>
                <label>缺陷描述
                  <textarea [(ngModel)]="defect.description" rows="2" placeholder="写明不符合的事实，便于补充材料逐条对应"></textarea>
                </label>
                <label>风险等级
                  <select [(ngModel)]="defect.riskLevel">
                    <option value="low">低</option><option value="medium">中</option>
                    <option value="high">高</option><option value="critical">严重</option>
                  </select>
                </label>
              </article>
              <p *ngIf="!defectDrafts.length" class="hint">至少填写一条缺陷。</p>
            </div>

            <footer class="drawer-footer">
              <button mat-button (click)="close.emit()">取消</button>
              <button mat-flat-button color="primary" [disabled]="busy" (click)="emitOpen()">发起整改</button>
            </footer>
          </div>

          <!-- 整改进行中 / 待复检 / 已通过 -->
          <div *ngIf="data" class="drawer-body">
            <section class="rect-summary">
              <div><span>状态</span><strong>{{ RECTIFICATION_STATUS_LABEL[data.checkStatus] }}</strong></div>
              <div><span>责任人</span><strong>{{ data.assignee }}</strong></div>
              <div><span>整改期限</span><strong [class.overdue]="remainingDays < 0 && data.checkStatus !== 'pass'">
                {{ formatDate(data.dueAt) }}<small *ngIf="data.checkStatus !== 'pass'">（剩余 {{ remainingText }}）</small><small *ngIf="data.checkStatus === 'pass'">已闭环</small>
              </strong></div>
              <div><span>整改进度</span><strong>{{ data.resolvedItems }} / {{ data.totalItems }} 条已关闭 · 第 {{ data.currentRound }} 轮</strong></div>
            </section>

            <section class="defect-list">
              <h3>缺陷清单与最新说明</h3>
              <article *ngFor="let item of data.items" class="defect-row" [class.resolved]="item.resolved">
                <div class="defect-head">
                  <span class="seq">#{{ item.seq }}</span>
                  <strong>{{ item.clause }}</strong>
                  <span [class]="'risk risk--' + item.riskLevel">{{ item.riskLevel }}</span>
                  <span class="tag" [class.tag--ok]="item.resolved">{{ item.resolved ? '第' + item.resolvedRound + '轮关闭' : '待整改' }}</span>
                </div>
                <p>{{ item.description }}</p>
                <small *ngIf="item.latestResponse" class="latest">最新说明（第{{ item.latestRound }}轮）：{{ item.latestResponse }}<br />凭证：{{ item.latestEvidence }}</small>
              </article>
            </section>

            <!-- 办理人提交整改说明与凭证 -->
            <section *ngIf="canSubmit()" class="round-editor">
              <h3>提交第 {{ data.currentRound + 1 }} 轮整改材料</h3>
              <label>本轮总体说明
                <textarea [(ngModel)]="submitNote" rows="2" placeholder="概述本轮整改情况"></textarea>
              </label>
              <label>整包凭证（可选）
                <input [(ngModel)]="submitEvidence" placeholder="minio://evidence/rect/r2-pack.zip" />
              </label>
              <article *ngFor="let response of responseDrafts" class="defect-card">
                <header><span>#{{ response.itemSeq }} {{ clauseOf(response.itemSeq) }}</span></header>
                <label>整改说明
                  <textarea [(ngModel)]="response.note" rows="2" placeholder="针对该缺陷说明整改措施"></textarea>
                </label>
                <label>凭证
                  <input [(ngModel)]="response.evidence" placeholder="minio://evidence/…" />
                </label>
              </article>
              <p *ngIf="!responseDrafts.length" class="hint">全部缺陷已关闭。</p>
              <footer class="inline-footer">
                <button mat-flat-button color="primary" [disabled]="busy || !responseDrafts.length" (click)="emitSubmit()">提交复检</button>
              </footer>
            </section>

            <!-- 复核员逐条复检 -->
            <section *ngIf="canReview()" class="round-editor">
              <h3>复检第 {{ data.currentRound }} 轮材料</h3>
              <p class="hint">勾选认可的缺陷；存在任一不认可时整轮退回，需填写原因，办理人继续整改。全部认可后核验转为通过。</p>
              <article *ngFor="let verdict of verdictDrafts" class="defect-card">
                <header>
                  <span>#{{ verdict.itemSeq }} {{ clauseOf(verdict.itemSeq) }}</span>
                  <label class="inline-check"><input type="checkbox" [(ngModel)]="verdict.accepted" /> 认可该事项</label>
                </header>
                <small class="latest">办理说明：{{ latestResponseNote(verdict.itemSeq) }}<br />凭证：{{ latestResponseEvidence(verdict.itemSeq) }}</small>
                <label *ngIf="!verdict.accepted">退回原因
                  <textarea [(ngModel)]="verdict.reason" rows="2" placeholder="说明为何不认可及继续整改的要求"></textarea>
                </label>
              </article>
              <footer class="inline-footer">
                <button mat-flat-button color="warn" class="warn-btn" [disabled]="busy || allAccepted()" (click)="emitReview(false)">退回继续整改</button>
                <button mat-flat-button color="primary" [disabled]="busy || !allAccepted()" (click)="emitReview(true)">全部认可，转为通过</button>
              </footer>
            </section>

            <!-- 每轮材料单独保留 -->
            <section class="round-history">
              <h3>历轮材料（{{ data.rounds.length }} 条，单独保留不覆盖）</h3>
              <article *ngFor="let round of data.rounds" class="round-row">
                <header>
                  <span class="tag" [class.tag--ok]="round.kind === 'approved'" [class.tag--warn]="round.kind === 'returned'">{{ ROUND_KIND_LABEL[round.kind] }}</span>
                  <strong>材料第 {{ round.submissionNo }} 轮</strong>
                  <small>{{ round.kind === 'submitted' ? round.submitter : round.reviewer }} · {{ formatDate(round.createdAt) }}</small>
                </header>
                <p *ngIf="round.note">{{ round.note }}</p>
                <p *ngIf="round.evidence" class="evidence-link">凭证：{{ round.evidence }}</p>
                <p *ngIf="round.reviewNote" class="return-reason">复核意见：{{ round.reviewNote }}（认可 {{ round.acceptedItems }} / 退回 {{ round.rejectedItems }}）</p>
                <ul *ngIf="round.responses.length">
                  <li *ngFor="let response of round.responses">
                    <strong>#{{ response.itemSeq }}</strong> {{ response.note }}
                    <small>凭证：{{ response.evidence }}<ng-container *ngIf="response.accepted !== null"> · {{ response.accepted ? '已认可' : '不认可：' + response.reviewNote }}</ng-container></small>
                  </li>
                </ul>
              </article>
            </section>
          </div>
        </ng-container>
      </section>
    </div>
  `,
})
export class RectificationDrawerComponent implements OnChanges {
  @Input({ required: true }) check!: DomainRecord;
  @Input() data: Rectification | null = null;
  @Input() loading = false;
  @Input() busy = false;
  @Input() error = '';
  @Output() close = new EventEmitter<void>();
  @Output() opened = new EventEmitter<void>();
  @Output() submitted = new EventEmitter<void>();
  @Output() reviewed = new EventEmitter<void>();
  @Output() requestError = new EventEmitter<string>();

  readonly auth = useAuth();
  readonly formatDate = formatDate;
  readonly RECTIFICATION_STATUS_LABEL = RECTIFICATION_STATUS_LABEL;

  openReason = '';
  assignee = '';
  dueAtLocal = '';
  defectDrafts: DefectDraft[] = [{ clause: '', description: '', riskLevel: 'medium' }];

  submitNote = '';
  submitEvidence = '';
  responseDrafts: ResponseDraft[] = [];

  verdictDrafts: VerdictDraft[] = [];
  private initializedKey = '';

  constructor(private readonly changeDetector: ChangeDetectorRef) {}

  ngOnChanges(): void {
    const status = this.data?.checkStatus ?? 'none';
    // Re-seed the working forms whenever the record, status or round changes so
    // a returned round presents a clean second submission and switching checks
    // never leaks drafts between records.
    const key = `${this.check?.id ?? 0}:${status}:${this.data?.currentRound ?? 0}`;
    if (key !== this.initializedKey) {
      this.initializedKey = key;
      this.responseDrafts = [];
      this.verdictDrafts = [];
      this.submitNote = '';
      this.submitEvidence = '';
      if (this.data?.checkStatus === 'rectifying' && this.auth.hasMinimumRole('operator')) {
        this.responseDrafts = this.openDefects().map((item) => ({ itemSeq: item.seq, note: '', evidence: '' }));
      }
      if (this.data?.checkStatus === 'recheck_pending' && this.auth.hasMinimumRole('reviewer')) {
        this.verdictDrafts = this.openDefects().map((item) => ({ itemSeq: item.seq, accepted: false, reason: '' }));
      }
    }
    if (!this.data) {
      this.openReason = '';
      this.assignee = '';
      this.dueAtLocal = this.defaultDue();
    }
    this.changeDetector.detectChanges();
  }

  canOpen(): boolean {
    return this.check.status === 'fail' && this.auth.hasMinimumRole('reviewer') && !this.data;
  }
  canSubmit(): boolean {
    return this.data?.checkStatus === 'rectifying' && this.auth.hasMinimumRole('operator');
  }
  canReview(): boolean {
    return this.data?.checkStatus === 'recheck_pending' && this.auth.hasMinimumRole('reviewer');
  }

  openDefects(): DefectItem[] {
    return (this.data?.items ?? []).filter((item) => !item.resolved);
  }
  clauseOf(seq: number): string {
    return this.data?.items.find((item) => item.seq === seq)?.clause ?? '';
  }
  latestResponseNote(seq: number): string {
    const item = this.data?.items.find((entry) => entry.seq === seq);
    return item?.latestResponse || '—';
  }
  latestResponseEvidence(seq: number): string {
    const item = this.data?.items.find((entry) => entry.seq === seq);
    return item?.latestEvidence || '—';
  }
  allAccepted(): boolean {
    return this.verdictDrafts.length > 0 && this.verdictDrafts.every((verdict) => verdict.accepted);
  }

  get remainingDays(): number {
    if (!this.data) return 0;
    return Math.ceil((new Date(this.data.dueAt).getTime() - Date.now()) / 86_400_000);
  }
  get remainingText(): string {
    const days = this.remainingDays;
    return days > 0 ? `${days} 天` : days === 0 ? '今日到期' : `已逾期 ${Math.abs(days)} 天`;
  }

  addDefect(): void {
    this.defectDrafts.push({ clause: '', description: '', riskLevel: 'medium' });
  }
  removeDefect(index: number): void {
    this.defectDrafts.splice(index, 1);
  }

  defaultDue(): string {
    const date = new Date(Date.now() + 3 * 86_400_000);
    const pad = (value: number) => String(value).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T18:00`;
  }

  emitOpen(): void {
    const items = this.defectDrafts
      .map((defect) => ({ clause: defect.clause.trim(), description: defect.description.trim(), riskLevel: defect.riskLevel }))
      .filter((defect) => defect.clause && defect.description);
    if (!this.openReason.trim() || this.openReason.trim().length < 3) return this.requestError.emit('请填写不合格原因（至少 3 个字）');
    if (!this.assignee.trim()) return this.requestError.emit('请填写整改责任人');
    if (!this.dueAtLocal) return this.requestError.emit('请选择整改期限');
    if (new Date(this.dueAtLocal).getTime() <= Date.now()) return this.requestError.emit('整改期限必须晚于当前时间');
    if (!items.length) return this.requestError.emit('至少逐条填写一条缺陷');
    (async () => {
      try {
        await openRectification(this.check.id, {
          expectedVersion: this.check.version,
          reason: this.openReason.trim(),
          assignee: this.assignee.trim(),
          dueAt: new Date(this.dueAtLocal).toISOString(),
          items,
        });
        this.opened.emit();
      } catch (error) {
        this.requestError.emit(error instanceof Error ? error.message : String(error));
      }
    })();
  }

  emitSubmit(): void {
    const responses = this.responseDrafts
      .map((response) => ({ itemSeq: response.itemSeq, note: response.note.trim(), evidence: response.evidence.trim() }))
      .filter((response) => response.note || response.evidence);
    if (!this.submitNote.trim() || this.submitNote.trim().length < 3) return this.requestError.emit('请填写本轮总体说明（至少 3 个字）');
    if (responses.length !== this.responseDrafts.length || responses.some((response) => response.note.length < 2 || response.evidence.length < 2)) {
      return this.requestError.emit('每条待整改缺陷都必须填写说明与凭证');
    }
    (async () => {
      try {
        await submitRectification(this.check.id, {
          expectedVersion: this.data!.version,
          note: this.submitNote.trim(),
          evidence: this.submitEvidence.trim(),
          responses,
        });
        this.submitted.emit();
      } catch (error) {
        this.requestError.emit(error instanceof Error ? error.message : String(error));
      }
    })();
  }

  emitReview(_approveAll: boolean): void {
    const verdicts = this.verdictDrafts.map((verdict) => ({
      itemSeq: verdict.itemSeq,
      accepted: verdict.accepted,
      reason: verdict.reason.trim(),
    }));
    const rejectedWithoutReason = verdicts.some((verdict) => !verdict.accepted && verdict.reason.length < 3);
    if (rejectedWithoutReason) return this.requestError.emit('退回时必须为每条不认可事项填写原因（至少 3 个字）');
    (async () => {
      try {
        await reviewRectification(this.check.id, { expectedVersion: this.data!.version, verdicts });
        this.reviewed.emit();
      } catch (error) {
        this.requestError.emit(error instanceof Error ? error.message : String(error));
      }
    })();
  }
}

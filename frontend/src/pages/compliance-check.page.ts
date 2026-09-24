import { AsyncPipe, CommonModule } from '@angular/common';
import { ChangeDetectorRef, Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MetricCardComponent } from '../components/common/metric-card.component';
import { StatusBadgeComponent } from '../components/common/status-badge.component';
import { useAuth } from '../hooks/use-auth';
import { createPagination } from '../hooks/use-pagination';
import { ComplianceCheckStore } from '../stores/compliance-check.store';
import type { ComplianceCheck, RemediationRound, RemediationRoundItem } from '../types/compliance';
import type { DomainRecord } from '../types/domain';
import { formatDate } from '../utils/format';

interface DefectDraft { description: string; category: string; evidence: string }
interface SubmissionDraft { roundItemId: number; note: string; evidenceUrls: string }
interface ReviewDraft { roundItemId: number; approved: boolean; comment: string }

@Component({
  selector: 'app-compliance-check-page',
  standalone: true,
  imports: [CommonModule, AsyncPipe, FormsModule, MatButtonModule, MetricCardComponent, StatusBadgeComponent],
  template: `
    <main class="workspace compliance-workspace" *ngIf="store.state$ | async as state">
      <header class="page-header">
        <div>
          <p class="eyebrow">业务工作台</p>
          <h1>合规核验与整改复检</h1>
          <p>失败后登记责任人和期限，按缺陷提交整改凭证；复核员逐项确认，全部通过后才转为核验通过。</p>
        </div>
        <button *ngIf="auth.hasMinimumRole('operator')" mat-flat-button color="primary" (click)="openCreate()">新增核验</button>
      </header>

      <section class="metrics">
        <app-metric-card label="核验总数" [value]="state.meta.total" detail="当前查询结果" />
        <app-metric-card label="待复检" [value]="countByStatus(state.items, 'pending_reinspection')" detail="已提交整改材料" />
        <app-metric-card label="整改中" [value]="countByStatus(state.items, 'fail')" detail="需要线下闭环前的线上跟踪" />
      </section>

      <section class="toolbar">
        <input matInput aria-label="搜索" [(ngModel)]="search" placeholder="搜索核验编码、联单编码或名称" (keyup.enter)="query()" />
        <button mat-flat-button color="primary" (click)="query()">查询</button>
        <button mat-button (click)="reset()">重置</button>
      </section>
      <div *ngIf="state.error" class="alert" role="alert">{{ state.error }}</div>

      <section class="table-shell">
        <table>
          <thead>
            <tr><th>核验 / 联单</th><th>状态</th><th>整改进度</th><th>责任人 / 剩余期限</th><th>最新说明</th><th>风险</th><th>更新时间</th><th>操作</th></tr>
          </thead>
          <tbody>
            <tr *ngFor="let item of state.items; trackBy: trackById">
              <td>
                <strong>{{ item.code }}</strong>
                <small>{{ item.name }} · {{ item.manifestCode }}</small>
              </td>
              <td><app-status-badge [status]="item.status" /><small>{{ item.decisionBasis || '待核验决定' }}</small></td>
              <td>
                <ng-container *ngIf="item.remediation; else noRemediation">
                  <strong>第 {{ item.remediation.currentRoundNo }} 轮 · {{ roundStatusLabel(item.remediation.roundStatus) }}</strong>
                  <small>通过 {{ item.remediation.approvedItems }}/{{ item.remediation.totalItems }} · 退回 {{ item.remediation.rejectedItems }}</small>
                  <div class="progress-line"><i [style.width.%]="progressPercent(item)"></i></div>
                </ng-container>
                <ng-template #noRemediation><span class="muted">尚未发起整改</span></ng-template>
              </td>
              <td>
                <ng-container *ngIf="item.remediation; else ownerOnly">
                  <strong>{{ item.remediation.assignee }}</strong>
                  <small [class.overdue]="remainingDays(item) < 0">
                    期限 {{ formatDate(item.remediation.dueAt) }} · {{ deadlineLabel(item) }}
                  </small>
                </ng-container>
                <ng-template #ownerOnly><small>{{ item.owner }}</small></ng-template>
              </td>
              <td class="note-cell">
                <ng-container *ngIf="item.remediation?.latestNote; else noNote">
                  <span>{{ item.remediation?.latestNote }}</span>
                  <small>{{ formatDate(item.remediation?.latestAt || '') }}</small>
                </ng-container>
                <ng-template #noNote><span class="muted">-</span></ng-template>
              </td>
              <td><span [class]="'risk risk--' + item.riskLevel">{{ item.riskLevel }}</span></td>
              <td>{{ formatDate(item.updatedAt) }}</td>
              <td class="actions">
                <button class="table-action" (click)="openDetail(item)">整改档案</button>
                <button *ngIf="isPending(item) && canReview()" class="table-action" (click)="openDecision(item, 'pass')">通过</button>
                <button *ngIf="isPending(item) && canReview()" class="table-action danger-action" (click)="openFailDecision(item)">判不合格</button>
                <button *ngIf="isPendingReinspection(item) && canReview()" class="table-action" (click)="openReview(item)">复检</button>
                <button *ngIf="canEscalate(item) && canReview()" class="table-action" (click)="escalate(item)">升级复核</button>
                <span *ngIf="item.status === 'pass' || item.status === 'escalated'" class="muted">流程结束</span>
              </td>
            </tr>
            <tr *ngIf="!state.items.length && !state.loading"><td colspan="8" class="empty">暂无记录</td></tr>
          </tbody>
        </table>
        <div *ngIf="state.loading" class="loading">正在同步核验与整改数据…</div>
      </section>

      <footer class="pager" *ngIf="state.meta.total > state.meta.pageSize">
        <span>第 {{ state.meta.page }} / {{ pagination.pages() }} 页</span>
        <button mat-button [disabled]="pagination.page() <= 1" (click)="previousPage()">上一页</button>
        <button mat-button [disabled]="pagination.page() >= pagination.pages()" (click)="nextPage()">下一页</button>
      </footer>

      <div class="modal-backdrop" *ngIf="showCreate" (click.self)="closeCreate()">
        <section class="modal" role="dialog" aria-modal="true">
          <h2>新增合规核验</h2>
          <p class="muted">演示创建会关联已提交联单 TM-002，并生成完整核验凭证字段。</p>
          <footer>
            <button mat-button (click)="closeCreate()">取消</button>
            <button mat-flat-button color="primary" (click)="createDemo()">确认创建</button>
          </footer>
        </section>
      </div>

      <div class="modal-backdrop" *ngIf="decisionTarget" (click.self)="closeDecision()">
        <section class="modal" role="dialog" aria-modal="true">
          <h2>核验通过</h2>
          <label>通过依据 <textarea [(ngModel)]="decisionReason" rows="4" placeholder="逐项说明核验清单均满足要求"></textarea></label>
          <footer>
            <button mat-button (click)="closeDecision()">取消</button>
            <button mat-flat-button color="primary" (click)="confirmPass()">确认通过</button>
          </footer>
        </section>
      </div>

      <div class="modal-backdrop" *ngIf="failTarget" (click.self)="closeFailDecision()">
        <section class="modal modal--wide" role="dialog" aria-modal="true">
          <h2>判为不合格并发起整改</h2>
          <div class="form-grid form-grid--two">
            <label>责任人 <input [(ngModel)]="assignee" placeholder="例如：张三 / 运行一组" /></label>
            <label>整改期限 <input type="datetime-local" [(ngModel)]="dueAtLocal" /></label>
          </div>
          <label>不合格总体原因 <textarea [(ngModel)]="decisionReason" rows="3" placeholder="说明本次核验结论及依据"></textarea></label>
          <section class="draft-list">
            <header><strong>缺陷清单</strong><button type="button" class="link-button" (click)="addDefect()">新增一条</button></header>
            <article *ngFor="let defect of defects; let index = index" class="draft-card">
              <div class="draft-card__title"><span>缺陷 {{ index + 1 }}</span><button type="button" class="link-button danger-action" (click)="removeDefect(index)">删除</button></div>
              <div class="form-grid form-grid--two">
                <label>缺陷分类 <input [(ngModel)]="defect.category" placeholder="证照 / 联单 / 称重 / 去向" /></label>
                <label>原始凭证 <input [(ngModel)]="defect.evidence" placeholder="原始证据编号或对象存储地址" /></label>
              </div>
              <label>缺陷描述 <textarea [(ngModel)]="defect.description" rows="2" placeholder="逐条写明不符合项，后续材料将对应此条"></textarea></label>
            </article>
          </section>
          <p class="conflict-hint">提交后第 1 轮整改立即生成；重复提交或旧版本会返回冲突，不会覆盖既有缺陷。</p>
          <footer>
            <button mat-button (click)="closeFailDecision()">取消</button>
            <button mat-flat-button color="primary" (click)="confirmFail()">发起整改</button>
          </footer>
        </section>
      </div>

      <div class="modal-backdrop" *ngIf="detailItem" (click.self)="closeDetail()">
        <section class="modal modal--large" role="dialog" aria-modal="true">
          <header class="detail-header">
            <div>
              <p class="eyebrow">整改复检档案</p>
              <h2>{{ detailItem.code }} · {{ detailItem.name }}</h2>
              <small>联单 {{ detailItem.manifestCode }} · 当前版本 V{{ detailItem.version }}</small>
            </div>
            <app-status-badge [status]="detailItem.status" />
          </header>

          <ng-container *ngIf="state.detail as detail">
            <section class="round-panel" *ngFor="let round of detail.rounds; trackBy: trackRoundById">
              <header>
                <div>
                  <strong>第 {{ round.roundNo }} 轮 · {{ roundStatusLabel(round.status) }}</strong>
                  <small>责任人 {{ round.assignee }} · 期限 {{ formatDate(round.dueAt) }}</small>
                </div>
                <span [class]="'round-state round-state--' + round.status">{{ roundStatusLabel(round.status) }}</span>
              </header>
              <p class="return-reason" *ngIf="round.returnReason">退回原因：{{ round.returnReason }}</p>
              <div class="item-list">
                <article class="item-card" *ngFor="let item of round.items; trackBy: trackItemById">
                  <div class="item-card__heading">
                    <strong>{{ item.itemNo }}. {{ item.description }}</strong>
                    <span [class]="'item-state item-state--' + item.status">{{ itemStatusLabel(item.status) }}</span>
                  </div>
                  <small>分类：{{ item.category || '未分类' }} · 原始凭证：{{ item.originalEvidence || '-' }}</small>
                  <div class="submission-list">
                    <div *ngFor="let submission of submissionsFor(round, item)" class="submission-record">
                      <strong>{{ submission.submittedBy }} · {{ formatDate(submission.submittedAt) }}</strong>
                      <p>{{ submission.note }}</p>
                      <a *ngFor="let url of submission.evidenceUrls" [href]="url" target="_blank" rel="noreferrer">{{ url }}</a>
                    </div>
                    <small *ngIf="!submissionsFor(round, item).length" class="muted">尚未提交本轮材料</small>
                  </div>
                  <small class="review-comment" *ngIf="item.reviewComment">复核意见：{{ item.reviewComment }}</small>
                </article>
              </div>

              <section *ngIf="activeRound(round) && detailItem.status === 'fail' && auth.hasMinimumRole('operator')" class="draft-list">
                <header><strong>提交本轮整改</strong><span class="muted">每条缺陷都要填写说明和至少一个凭证</span></header>
                <article *ngFor="let draft of submissionDrafts(round); let index = index" class="draft-card">
                  <strong>{{ draft.roundItemId ? itemLabel(round, draft.roundItemId) : '缺陷 ' + (index + 1) }}</strong>
                  <label>整改说明 <textarea [ngModel]="draft.note" (ngModelChange)="draft.note = $event" rows="2" placeholder="说明整改措施、完成情况"></textarea></label>
                  <label>凭证地址（多个用换行或逗号分隔） <textarea [ngModel]="draft.evidenceUrls" (ngModelChange)="draft.evidenceUrls = $event" rows="2" placeholder="minio://evidence/... 或 https://..."></textarea></label>
                </article>
                <button mat-flat-button color="primary" (click)="confirmSubmit(round)">提交进入待复检</button>
              </section>

              <section *ngIf="activeRound(round) && detailItem.status === 'pending_reinspection' && auth.hasMinimumRole('reviewer')" class="draft-list">
                <header><strong>逐项复检</strong><span class="muted">全部认可才可以通过；任一项退回都会保留旧材料并开启下一轮。</span></header>
                <article *ngFor="let draft of reviewDrafts(round)" class="draft-card review-draft">
                  <label class="inline-check">
                    <input type="checkbox" [ngModel]="draft.approved" (ngModelChange)="draft.approved = $event" />
                    <strong>{{ itemLabel(round, draft.roundItemId) }} 已认可</strong>
                  </label>
                  <label>复核意见（退回必填） <textarea [ngModel]="draft.comment" (ngModelChange)="draft.comment = $event" rows="2" placeholder="不认可时说明原因"></textarea></label>
                </article>
                <label>复检结论说明 <textarea [(ngModel)]="reviewReason" rows="2"></textarea></label>
                <div class="modal-actions">
                  <button mat-flat-button color="warn" (click)="confirmReview(round, false)">退回继续整改</button>
                  <button mat-flat-button color="primary" (click)="confirmReview(round, true)">全部认可并通过</button>
                </div>
              </section>
            </section>
            <div class="empty" *ngIf="!detail.rounds.length">该核验暂未发起整改；复核判为不合格时需逐条登记缺陷。</div>
          </ng-container>
          <footer><button mat-button (click)="closeDetail()">关闭</button></footer>
        </section>
      </div>
    </main>
  `,
})
export class ComplianceCheckPageComponent implements OnInit {
  readonly auth = useAuth();
  readonly formatDate = formatDate;
  readonly pagination = createPagination(() => this.store.snapshot.meta.total, 10);
  search = '';
  showCreate = false;
  decisionTarget: ComplianceCheck | null = null;
  failTarget: ComplianceCheck | null = null;
  detailItem: ComplianceCheck | null = null;
  decisionReason = '';
  assignee = '';
  dueAtLocal = '';
  defects: DefectDraft[] = [this.emptyDefect()];
  reviewReason = '';
  private submissionDraftMap = new Map<number, SubmissionDraft[]>();
  private reviewDraftMap = new Map<number, ReviewDraft[]>();

  constructor(readonly store: ComplianceCheckStore, private readonly changeDetector: ChangeDetectorRef) {}

  async ngOnInit(): Promise<void> {
    await this.load();
  }

  trackById(_index: number, item: ComplianceCheck): number { return item.id; }
  trackRoundById(_index: number, round: RemediationRound): number { return round.id; }
  trackItemById(_index: number, item: RemediationRoundItem): number { return item.id; }
  isPending(item: ComplianceCheck): boolean { return item.status === 'pending'; }
  isPendingReinspection(item: ComplianceCheck): boolean { return item.status === 'pending_reinspection'; }
  canReview(): boolean { return this.auth.hasMinimumRole('reviewer'); }
  canEscalate(item: ComplianceCheck): boolean { return item.status === 'fail'; }
  countByStatus(items: ComplianceCheck[], status: string): number { return items.filter((item) => item.status === status).length; }

  async query(): Promise<void> { this.pagination.reset(); await this.load(); }
  async reset(): Promise<void> { this.search = ''; this.pagination.reset(); await this.load(); }
  async previousPage(): Promise<void> { this.pagination.previous(); await this.load(); }
  async nextPage(): Promise<void> { this.pagination.next(); await this.load(); }

  openCreate(): void { this.showCreate = true; }
  closeCreate(): void { this.showCreate = false; }

  async createDemo(): Promise<void> {
    const now = Date.now();
    const input: Partial<DomainRecord> = {
      code: `CC-${String(now).slice(-6)}`,
      name: '新增合规核验',
      description: '通过整改复检工作台创建的核验记录',
      manifestCode: 'TM-002',
      checklist: '产废许可、承运资质、联单数量、处置去向',
      decisionBasis: '',
      facility: '东区危废暂存区', owner: '现场操作员', category: '危废转运', riskLevel: 'medium',
      metricValue: 25, metricUnit: 'score', effectiveAt: new Date().toISOString(),
      evidence: `minio://evidence/checks/${now}.pdf`, relatedCode: '',
    };
    try {
      await this.store.create(input);
      this.showCreate = false;
    } catch { /* store surfaces request errors */ }
    this.changeDetector.detectChanges();
  }

  openDecision(item: ComplianceCheck, _status: 'pass'): void {
    this.decisionTarget = item;
    this.decisionReason = '';
  }
  closeDecision(): void { this.decisionTarget = null; }

  async confirmPass(): Promise<void> {
    if (!this.decisionTarget) return;
    try {
      await this.store.decide(this.decisionTarget.id, {
        status: 'pass',
        expectedVersion: this.decisionTarget.version,
        reason: this.decisionReason.trim() || '核验清单及补充材料全部满足要求',
      });
      this.decisionTarget = null;
    } catch { /* store surfaces request errors */ }
    this.changeDetector.detectChanges();
  }

  openFailDecision(item: ComplianceCheck): void {
    this.failTarget = item;
    this.decisionReason = '';
    this.assignee = item.owner || '';
    const due = new Date(Date.now() + 7 * 86_400_000);
    this.dueAtLocal = this.toLocalInput(due);
    this.defects = [this.emptyDefect()];
  }
  closeFailDecision(): void { this.failTarget = null; }
  addDefect(): void { this.defects.push(this.emptyDefect()); }
  removeDefect(index: number): void { this.defects.splice(index, 1); }

  async confirmFail(): Promise<void> {
    if (!this.failTarget) return;
    const dueAt = this.dueAtLocal ? new Date(this.dueAtLocal).toISOString() : '';
    try {
      await this.store.decide(this.failTarget.id, {
        status: 'fail',
        expectedVersion: this.failTarget.version,
        reason: this.decisionReason.trim(),
        assignee: this.assignee.trim(),
        dueAt,
        defects: this.defects.map((defect) => ({
          description: defect.description.trim(),
          category: defect.category.trim(),
          evidence: defect.evidence.trim(),
        })),
      });
      this.failTarget = null;
    } catch { /* store surfaces request errors */ }
    this.changeDetector.detectChanges();
  }

  async escalate(item: ComplianceCheck): Promise<void> {
    try {
      await this.store.decide(item.id, { status: 'escalated', expectedVersion: item.version, reason: '人工升级至上级复核处理' });
    } catch { /* store surfaces request errors */ }
    this.changeDetector.detectChanges();
  }

  async openDetail(item: ComplianceCheck): Promise<void> {
    this.detailItem = item;
    this.submissionDraftMap.clear();
    this.reviewDraftMap.clear();
    this.reviewReason = '';
    await this.store.loadRemediation(item.id);
    this.changeDetector.detectChanges();
  }

  closeDetail(): void {
    this.detailItem = null;
    this.submissionDraftMap.clear();
    this.reviewDraftMap.clear();
    this.store.clearDetail();
  }

  activeRound(round: RemediationRound): boolean {
    const rounds = this.store.snapshot.detail?.rounds || [];
    return Boolean(rounds.length && rounds[rounds.length - 1].id === round.id);
  }

  submissionDrafts(round: RemediationRound): SubmissionDraft[] {
    const existing = this.submissionDraftMap.get(round.id);
    if (existing) return existing;
    const drafts = round.items
      .filter((item) => item.status === 'pending')
      .map((item) => ({ roundItemId: item.id, note: '', evidenceUrls: '' }));
    this.submissionDraftMap.set(round.id, drafts);
    return drafts;
  }

  reviewDrafts(round: RemediationRound): ReviewDraft[] {
    const existing = this.reviewDraftMap.get(round.id);
    if (existing) return existing;
    const drafts = round.items.map((item) => ({ roundItemId: item.id, approved: true, comment: '' }));
    this.reviewDraftMap.set(round.id, drafts);
    return drafts;
  }

  async confirmSubmit(round: RemediationRound): Promise<void> {
    if (!this.detailItem) return;
    const drafts = this.submissionDrafts(round);
    try {
      await this.store.submit(this.detailItem.id, round.id, {
        expectedVersion: this.detailItem.version,
        roundVersion: round.version,
        items: drafts.map((draft) => ({
          roundItemId: draft.roundItemId,
          note: draft.note.trim(),
          evidenceUrls: this.parseEvidence(draft.evidenceUrls),
        })),
      });
      this.submissionDraftMap.delete(round.id);
      this.syncDetailItem();
    } catch { /* store surfaces request errors */ }
    this.changeDetector.detectChanges();
  }

  async confirmReview(round: RemediationRound, approve: boolean): Promise<void> {
    if (!this.detailItem) return;
    const drafts = this.reviewDrafts(round);
    try {
      await this.store.review(this.detailItem.id, round.id, {
        action: approve ? 'approve' : 'return',
        expectedVersion: this.detailItem.version,
        roundVersion: round.version,
        reason: this.reviewReason.trim(),
        items: drafts.map((draft) => ({
          roundItemId: draft.roundItemId,
          approved: draft.approved,
          comment: draft.comment.trim(),
        })),
      });
      this.reviewDraftMap.delete(round.id);
      this.syncDetailItem();
    } catch { /* store surfaces request errors */ }
    this.changeDetector.detectChanges();
  }

  submissionsFor(round: RemediationRound, item: RemediationRoundItem) {
    return round.submissions.filter((submission) => submission.roundItemId === item.id);
  }

  itemLabel(round: RemediationRound, id: number): string {
    const item = round.items.find((candidate) => candidate.id === id);
    return item ? `缺陷 ${item.itemNo}` : `事项 ${id}`;
  }

  progressPercent(item: ComplianceCheck): number {
    const remediation = item.remediation;
    if (!remediation || remediation.totalItems === 0) return 0;
    return Math.round((remediation.approvedItems / remediation.totalItems) * 100);
  }

  remainingDays(item: ComplianceCheck): number {
    if (!item.remediation) return 0;
    return Math.ceil((new Date(item.remediation.dueAt).getTime() - Date.now()) / 86_400_000);
  }

  deadlineLabel(item: ComplianceCheck): string {
    if (item.remediation?.roundStatus === 'passed') return '复检已通过';
    if (item.remediation?.roundStatus === 'returned') return '已退回';
    const days = this.remainingDays(item);
    if (days < 0) return `已逾期 ${Math.abs(days)} 天`;
    if (days === 0) return '今日到期';
    return `剩余 ${days} 天`;
  }

  roundStatusLabel(status: string): string {
    const labels: Record<string, string> = {
      rectifying: '整改中', pending_reinspection: '待复检', returned: '已退回', passed: '复检通过',
    };
    return labels[status] || status;
  }

  itemStatusLabel(status: string): string {
    const labels: Record<string, string> = {
      pending: '待整改', submitted: '待复检', approved: '已认可', rejected: '已退回',
    };
    return labels[status] || status;
  }

  private async load(): Promise<void> {
    await this.store.load(this.search, this.pagination.page(), this.pagination.pageSize());
    this.changeDetector.detectChanges();
  }

  private syncDetailItem(): void {
    this.detailItem = this.store.snapshot.items.find((item) => item.id === this.detailItem?.id) || this.detailItem;
  }

  private emptyDefect(): DefectDraft {
    return { description: '', category: '', evidence: '' };
  }

  private parseEvidence(value: string): string[] {
    return value.split(/[\n,，]/).map((item) => item.trim()).filter(Boolean);
  }

  private toLocalInput(date: Date): string {
    const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
    return local.toISOString().slice(0, 16);
  }
}

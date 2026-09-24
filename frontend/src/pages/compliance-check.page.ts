import { AsyncPipe, CommonModule } from '@angular/common';
import { ChangeDetectorRef, Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { useAuth } from '../hooks/use-auth';
import { createPagination } from '../hooks/use-pagination';
import { ComplianceCheckStore } from '../stores/compliance-check.store';
import { getRectification, listRectificationSummaries } from '../api/rectification';
import { ConfirmDialogComponent } from '../components/common/confirm-dialog.component';
import { MetricCardComponent } from '../components/common/metric-card.component';
import { RectificationDrawerComponent } from '../components/common/rectification-drawer.component';
import { StatusBadgeComponent } from '../components/common/status-badge.component';
import type { DomainRecord } from '../types/domain';
import type { Rectification, RectificationSummary } from '../types/rectification';
import { formatDate } from '../utils/format';

@Component({
  selector: 'app-compliance-check-page',
  standalone: true,
  imports: [CommonModule, AsyncPipe, FormsModule, MatButtonModule, StatusBadgeComponent, MetricCardComponent, ConfirmDialogComponent, RectificationDrawerComponent],
  template: `
    <main class="workspace" *ngIf="store.state$ | async as state">
      <header class="page-header">
        <div>
          <p class="eyebrow">业务工作台</p>
          <h1>合规核验</h1>
          <p>基于联单证据形成核验决定；不合格事项在线发起整改与复检，逐轮留痕。</p>
        </div>
        <button *ngIf="auth.hasMinimumRole('operator')" mat-flat-button color="primary" (click)="openCreate()">新增合规核验</button>
      </header>

      <section class="metrics">
        <app-metric-card label="记录总数" [value]="state.meta.total" detail="当前查询结果" />
        <app-metric-card label="整改中/待复检" [value]="activeRectifications" detail="需要跟踪办理或复检" />
        <app-metric-card label="即将到期" [value]="overdueSoon" detail="剩余期限不足 2 天" />
      </section>

      <section class="toolbar">
        <input matInput aria-label="搜索" [(ngModel)]="search" placeholder="搜索核验编码或名称" (keyup.enter)="query()" />
        <button mat-flat-button color="primary" (click)="query()">查询</button>
        <button mat-button (click)="reset()">重置</button>
      </section>
      <div *ngIf="state.error" class="alert" role="alert">{{ state.error }}</div>

      <section class="table-shell">
        <table>
          <thead><tr>
            <th>编码</th><th>名称/联单</th><th>状态</th><th>整改进度</th><th>责任人/期限</th><th>最新说明</th><th>更新时间</th><th>操作</th>
          </tr></thead>
          <tbody>
            <tr *ngFor="let item of state.items; trackBy: trackById">
              <td><strong>{{ item.code }}</strong></td>
              <td>{{ item.name }}<small>{{ item.manifestCode }}</small></td>
              <td><app-status-badge [status]="item.status" /></td>
              <td>
                <ng-container *ngIf="summaryOf(item.id) as summary; else noRect">
                  <span class="rect-progress">{{ summary.resolvedItems }}/{{ summary.totalItems }} 条已关闭</span>
                  <small>第 {{ summary.currentRound }} 轮材料</small>
                </ng-container>
                <ng-template #noRect><span class="muted">{{ item.status === 'fail' ? '待发起整改' : '—' }}</span></ng-template>
              </td>
              <td>
                <ng-container *ngIf="summaryOf(item.id) as summary">
                  {{ summary.assignee }}
                  <small [class.overdue]="daysLeft(summary.dueAt) < 0 && summary.status !== 'pass'">{{ deadlineText(summary) }}</small>
                </ng-container>
                <small *ngIf="!summaryOf(item.id)">{{ item.owner }}</small>
              </td>
              <td class="latest-cell">
                <ng-container *ngIf="summaryOf(item.id) as summary">
                  <span class="latest-note">{{ summary.latestNote || '等待办理人提交' }}</span>
                </ng-container>
                <small *ngIf="!summaryOf(item.id)">{{ item.decisionBasis || '待决定' }}</small>
              </td>
              <td>{{ formatDate(item.updatedAt) }}</td>
              <td class="actions">
                <button class="table-action" (click)="openRectification(item)">整改复检</button>
                <ng-container *ngIf="auth.hasMinimumRole('reviewer')">
                  <button *ngFor="let target of transitions(item)" class="table-action" (click)="openTransition(item, target)">{{ transitionLabel(target) }}</button>
                </ng-container>
              </td>
            </tr>
            <tr *ngIf="!state.items.length && !state.loading"><td colspan="8" class="empty">暂无记录</td></tr>
          </tbody>
        </table>
        <div *ngIf="state.loading" class="loading">正在同步业务数据…</div>
      </section>

      <footer class="pager" *ngIf="state.meta.total > state.meta.pageSize">
        <span>第 {{ state.meta.page }} / {{ pagination.pages() }} 页</span>
        <button mat-button [disabled]="pagination.page() <= 1" (click)="previousPage()">上一页</button>
        <button mat-button [disabled]="pagination.page() >= pagination.pages()" (click)="nextPage()">下一页</button>
      </footer>

      <app-confirm-dialog [open]="showCreate" title="新增合规核验" (cancel)="closeCreate()" (confirm)="createDemo()">
        <p>确认创建一条包含责任人、联单关联、风险和证据信息的核验记录。</p>
      </app-confirm-dialog>
      <app-confirm-dialog [open]="!!pending" title="确认状态迁移" (cancel)="closeTransition()" (confirm)="confirmTransition()">
        <p>状态迁移会校验关联联单，并与请求 ID 审计记录在同一事务中保存。</p>
        <strong>{{ pending?.item?.status }} → {{ pending?.status }}</strong>
        <label class="reason-field">决定依据 / 理由（必填）
          <textarea rows="2" [(ngModel)]="transitionReason" placeholder="写明判定依据，不合格时将据此发起整改"></textarea>
        </label>
      </app-confirm-dialog>

      <app-rectification-drawer
        *ngIf="drawerCheck"
        [check]="drawerCheck"
        [data]="drawerData"
        [loading]="drawerLoading"
        [busy]="false"
        [error]="drawerError"
        (close)="closeRectification()"
        (opened)="onRectificationChanged()"
        (submitted)="onRectificationChanged()"
        (reviewed)="onRectificationChanged()"
        (requestError)="drawerError = $event"
      />
    </main>
  `,
})
export class ComplianceCheckPage implements OnInit {
  readonly auth = useAuth();
  readonly formatDate = formatDate;
  readonly pagination = createPagination(() => this.store.snapshot.meta.total ?? 0, 10);
  search = '';
  showCreate = false;
  pending: { item: DomainRecord; status: string } | null = null;
  transitionReason = '';
  private summaries: Record<number, RectificationSummary> = {};

  drawerCheck: DomainRecord | null = null;
  drawerData: Rectification | null = null;
  drawerLoading = false;
  drawerError = '';

  constructor(readonly store: ComplianceCheckStore, private readonly changeDetector: ChangeDetectorRef) {}

  async ngOnInit(): Promise<void> {
    await this.load();
  }

  trackById(_index: number, item: DomainRecord): number {
    return item.id;
  }

  transitions(item: DomainRecord): readonly string[] {
    const graph: Record<string, readonly string[]> = {
      pending: ['pass', 'fail'],
      pass: [],
      fail: ['escalated'],
      rectifying: ['escalated'],
      recheck_pending: ['escalated'],
      escalated: [],
    };
    return graph[item.status] ?? [];
  }

  transitionLabel(status: string): string {
    const labels: Record<string, string> = { pass: '通过', fail: '不通过', escalated: '升级复核' };
    return labels[status] || status;
  }

  summaryOf(id: number): RectificationSummary | null {
    return this.summaries[id] ?? null;
  }

  get activeRectifications(): number {
    return Object.values(this.summaries).filter((summary) => summary.status === 'rectifying' || summary.status === 'recheck_pending').length;
  }

  get overdueSoon(): number {
    return Object.values(this.summaries).filter((summary) => {
      if (summary.status === 'pass') return false;
      const days = this.daysLeft(summary.dueAt);
      return days < 2;
    }).length;
  }

  daysLeft(value: string): number {
    return Math.ceil((new Date(value).getTime() - Date.now()) / 86_400_000);
  }

  deadlineText(summary: RectificationSummary): string {
    const days = this.daysLeft(summary.dueAt);
    const base = `期限 ${formatDate(summary.dueAt)}`;
    if (summary.status === 'pass') return base;
    if (days < 0) return `${base} · 已逾期 ${Math.abs(days)} 天`;
    if (days === 0) return `${base} · 今日到期`;
    return `${base} · 剩 ${days} 天`;
  }

  async query(): Promise<void> {
    this.pagination.reset();
    await this.load();
  }
  async reset(): Promise<void> {
    this.search = '';
    this.pagination.reset();
    await this.load();
  }
  async previousPage(): Promise<void> {
    this.pagination.previous();
    await this.load();
  }
  async nextPage(): Promise<void> {
    this.pagination.next();
    await this.load();
  }

  openCreate(): void {
    this.showCreate = true;
  }
  closeCreate(): void {
    this.showCreate = false;
  }
  openTransition(item: DomainRecord, status: string): void {
    this.pending = { item, status };
    this.transitionReason = '';
  }
  closeTransition(): void {
    this.pending = null;
    this.transitionReason = '';
  }

  async createDemo(): Promise<void> {
    const now = Date.now();
    try {
      await this.store.createRecord('checks', {
        code: `COMPLIANCECHECK-${String(now).slice(-6)}`,
        name: '新增合规核验',
        description: '通过合规工作台创建的业务记录',
        facility: '复核中心', owner: '现场操作员', category: '联单复核', riskLevel: 'medium',
        metricValue: 88, metricUnit: 'score', effectiveAt: new Date().toISOString(),
        evidence: `minio://evidence/checks/${now}.pdf`, relatedCode: '',
        manifestCode: 'TM-002', checklist: '产废许可、承运资质、联单数量、处置去向', decisionBasis: '',
      });
      this.showCreate = false;
    } catch {
      /* Store exposes the request error in its observable state. */
    } finally {
      this.changeDetector.detectChanges();
    }
  }

  async confirmTransition(): Promise<void> {
    if (!this.pending) return;
    const reason = this.transitionReason.trim();
    if (reason.length < 3) {
      this.store.notifyError('请填写决定依据（至少 3 个字）');
      this.changeDetector.detectChanges();
      return;
    }
    try {
      await this.store.transition('checks', this.pending.item, this.pending.status, reason);
      this.pending = null;
      this.transitionReason = '';
      await this.load();
    } catch {
      /* Store exposes the request error in its observable state. */
    } finally {
      this.changeDetector.detectChanges();
    }
  }

  async openRectification(item: DomainRecord): Promise<void> {
    this.drawerCheck = item;
    this.drawerData = null;
    this.drawerError = '';
    this.drawerLoading = true;
    this.changeDetector.detectChanges();
    try {
      const result = await getRectification(item.id);
      this.drawerData = result.data;
    } catch (error) {
      // 尚未发起整改时抽屉展示发起表单；其它错误展示出来。
      const message = error instanceof Error ? error.message : String(error);
      if (!/not_found|404/.test(message)) this.drawerError = message;
    } finally {
      this.drawerLoading = false;
      this.changeDetector.detectChanges();
    }
  }

  closeRectification(): void {
    this.drawerCheck = null;
    this.drawerData = null;
    this.drawerError = '';
  }

  async onRectificationChanged(): Promise<void> {
    await this.load();
    if (this.drawerCheck) {
      const refreshed = this.store.snapshot.items.find((item) => item.id === this.drawerCheck?.id) ?? this.drawerCheck;
      this.drawerCheck = refreshed;
      await this.openRectification(refreshed);
    }
  }

  private async load(): Promise<void> {
    await this.store.load('checks', this.search, this.pagination.page(), this.pagination.pageSize());
    const ids = this.store.snapshot.items.map((item) => item.id);
    if (ids.length) {
      try {
        const result = await listRectificationSummaries(ids);
        this.summaries = {};
        for (const [key, value] of Object.entries(result.data)) {
          this.summaries[Number(key)] = value;
        }
      } catch {
        this.summaries = {};
      }
    } else {
      this.summaries = {};
    }
    this.changeDetector.detectChanges();
  }
}

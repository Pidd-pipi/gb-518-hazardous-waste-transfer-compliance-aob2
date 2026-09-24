import { Component, Input } from '@angular/core';
import { statusTone } from '../../utils/format';

@Component({
  selector: 'app-status-badge',
  standalone: true,
  template: `<span [class]="'status status--' + tone">{{ label }}</span>`,
})
export class StatusBadgeComponent {
  @Input({ required: true }) status = '';

  get tone() { return statusTone(this.status); }

  get label(): string {
    const labels: Record<string, string> = {
      pending: '待处理',
      pass: '通过',
      fail: '不合格',
      pending_reinspection: '待复检',
      escalated: '已升级',
      submitted: '已提交',
      in_transit: '运输中',
      received: '已签收',
      rejected: '已驳回',
      active: '有效',
      restricted: '受限',
      suspended: '停用',
      expired: '已到期',
      verified: '已核准',
    };
    return labels[this.status] || this.status.replaceAll('_', ' ');
  }
}

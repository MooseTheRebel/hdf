import { cli } from '../wailsjs/go/models';
import { msg } from '@locales';
import { escapeHtml, html } from './i18n';

export function renderReportIssueResult(result: cli.ReportIssueResult): string {
    return `<p class="init-result-message">${html(msg.reportIssue.written, {}, {path: `<code>${escapeHtml(result.path)}</code>`})}</p>`;
}

import { cli } from '../wailsjs/go/models';

function escapeHtml(s: string): string {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

export function renderReportIssueResult(result: cli.ReportIssueResult): string {
    return `<p class="init-result-message">Report written to <code>${escapeHtml(result.path)}</code></p>`;
}

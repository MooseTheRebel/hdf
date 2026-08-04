import { cli } from '../wailsjs/go/models';
import { renderDiffContent } from './diff';

function escapeHtml(s: string): string {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

export function renderPendingWarnings(warnings: string[]): string {
    const rows = warnings.map(w => `<div class="link-warning-row">${escapeHtml(w)}</div>`).join('');
    return `
        <p>The hdf daemon has recorded the following warnings:</p>
        <div class="link-warning-list">${rows}</div>
        <p>Continue anyway?</p>
    `;
}

export function renderIncomingFileReview(file: cli.IncomingFile, index: number, total: number): string {
    return `
        <div class="link-review-counter">File ${index + 1} of ${total}</div>
        <div class="link-review-path">${escapeHtml(file.path)}</div>
        <div class="link-review-diff">${renderDiffContent(file.diff)}</div>
    `;
}

export function renderLinkResults(message: string, results: cli.LinkedFile[]): string {
    const messageHtml = message
        ? `<p class="link-results-message">${escapeHtml(message)}</p>`
        : '';
    const rows = results.map(r => {
        if (r.error) {
            return `<div class="link-result-row link-result-error"><span class="link-result-path">${escapeHtml(r.path)}</span><span class="link-result-status">${escapeHtml(r.error)}</span></div>`;
        }
        return `<div class="link-result-row link-result-ok"><span class="link-result-path">${escapeHtml(r.path)}</span><span class="link-result-status">linked</span></div>`;
    }).join('');
    return `
        ${messageHtml}
        <div class="link-results-list">${rows || '<div class="link-results-empty">No managed files.</div>'}</div>
    `;
}

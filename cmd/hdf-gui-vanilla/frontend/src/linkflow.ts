import { cli } from '../wailsjs/go/models';
import { msg } from '@locales';
import { renderDiffContent } from './diff';
import { escapeHtml, html } from './i18n';

export function renderPendingWarnings(warnings: string[]): string {
    const rows = warnings.map(w => `<div class="link-warning-row">${escapeHtml(w)}</div>`).join('');
    return `
        <p>${html(msg.warnings.intro)}</p>
        <div class="link-warning-list">${rows}</div>
        <p>${html(msg.warnings.prompt)}</p>
    `;
}

export function renderIncomingFileReview(file: cli.IncomingFile, index: number, total: number): string {
    return `
        <div class="link-review-counter">${html(msg.common.reviewCounter, {index: index + 1, total})}</div>
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
        return `<div class="link-result-row link-result-ok"><span class="link-result-path">${escapeHtml(r.path)}</span><span class="link-result-status">${html(msg.link.linked)}</span></div>`;
    }).join('');
    return `
        ${messageHtml}
        <div class="link-results-list">${rows || `<div class="link-results-empty">${html(msg.common.noManagedFiles)}</div>`}</div>
    `;
}

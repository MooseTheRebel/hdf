import { cli } from '../wailsjs/go/models';
import { msg } from '@locales';
import { renderDiffContent } from './diff';
import { escapeHtml, html } from './i18n';

export function renderPreservedFiles(files: cli.PreservedFile[]): string {
    const rows = files.map(f => `<div class="link-warning-row">${escapeHtml(f.path)}</div>`).join('');
    return `
        <p>${html(msg.promote.preservedIntro)}</p>
        <div class="link-warning-list">${rows}</div>
        <p>${html(msg.promote.preservedPrompt)}</p>
    `;
}

export function renderDivergedFileReview(file: cli.DivergedFile, index: number, total: number): string {
    return `
        <div class="link-review-counter">${html(msg.common.reviewCounter, {index: index + 1, total})}</div>
        <div class="link-review-path">${escapeHtml(file.path)}</div>
        <div class="link-review-diff">${renderDiffContent(file.diff)}</div>
    `;
}

export function renderPromoteResult(result: cli.PromoteResult): string {
    return `<p class="init-result-message">${escapeHtml(result.message)}</p>`;
}

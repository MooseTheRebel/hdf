import { cli } from '../wailsjs/go/models';
import { renderDiffContent } from './diff';

function escapeHtml(s: string): string {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

export function renderPreservedFiles(files: cli.PreservedFile[]): string {
    const rows = files.map(f => `<div class="link-warning-row">${escapeHtml(f.path)}</div>`).join('');
    return `
        <p>main has file(s) promoted by other machines that you haven't pulled. They will be preserved by promote:</p>
        <div class="link-warning-list">${rows}</div>
        <p>Continue promoting?</p>
    `;
}

export function renderDivergedFileReview(file: cli.DivergedFile, index: number, total: number): string {
    return `
        <div class="link-review-counter">File ${index + 1} of ${total}</div>
        <div class="link-review-path">${escapeHtml(file.path)}</div>
        <div class="link-review-diff">${renderDiffContent(file.diff)}</div>
    `;
}

export function renderPromoteResult(result: cli.PromoteResult): string {
    return `<p class="init-result-message">${escapeHtml(result.message)}</p>`;
}

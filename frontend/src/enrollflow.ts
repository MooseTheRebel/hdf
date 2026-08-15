import { cli } from '../wailsjs/go/models';
import { renderDiffContent } from './diff';

function escapeHtml(s: string): string {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

export function renderEnrollPreview(info: cli.EnrollStartInfo): string {
    const pathHtml = `<div class="enroll-preview-path">${escapeHtml(info.path)}</div>`;
    if (info.isNewFile) {
        return `${pathHtml}<p class="enroll-preview-note">This is a new file.</p>`;
    }
    if (!info.diff) {
        return `${pathHtml}<p class="enroll-preview-note">There are no changes to enroll.</p>`;
    }
    return `${pathHtml}<div class="enroll-preview-diff">${renderDiffContent(info.diff)}</div>`;
}

export function renderEnrollResult(result: cli.EnrollResult): string {
    return `<p class="enroll-result-message">${escapeHtml(result.message)}</p>`;
}

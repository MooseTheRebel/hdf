import { cli } from '../wailsjs/go/models';
import { msg } from '@locales';
import { renderDiffContent } from './diff';
import { escapeHtml, html } from './i18n';

export function renderEnrollPreview(info: cli.EnrollStartInfo): string {
    const pathHtml = `<div class="enroll-preview-path">${escapeHtml(info.path)}</div>`;
    if (info.isNewFile) {
        return `${pathHtml}<p class="enroll-preview-note">${html(msg.enroll.newFile)}</p>`;
    }
    if (!info.diff) {
        return `${pathHtml}<p class="enroll-preview-note">${html(msg.enroll.noChanges)}</p>`;
    }
    return `${pathHtml}<div class="enroll-preview-diff">${renderDiffContent(info.diff)}</div>`;
}

export function renderEnrollResult(result: cli.EnrollResult): string {
    return `<p class="enroll-result-message">${escapeHtml(result.message)}</p>`;
}

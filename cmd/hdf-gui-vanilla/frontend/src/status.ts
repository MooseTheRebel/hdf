import { cli } from '../wailsjs/go/models';
import { msg } from '@locales';
import { escapeHtml, html } from './i18n';

export function renderStatus(info: cli.StatusInfo): string {
    const fileRows = info.files.map(f => `
        <div class="status-file-row">
            <span class="status-file-path">${escapeHtml(f.path)}</span>
            <span class="status-file-state">${escapeHtml(f.status)}</span>
        </div>
    `).join('');

    const field = (label: string, value: string) =>
        `<div class="status-field"><span class="status-label">${html(label)}</span><span class="status-value">${escapeHtml(value)}</span></div>`;

    return `
        <div class="status-summary">
            ${field(msg.status.gitPushTarget, info.git_push_target)}
            ${field(msg.status.localDotfilesDir, info.local_dotfiles_dir)}
            ${field(msg.status.branch, info.branch)}
            ${field(msg.status.lastCommit, info.last_commit)}
            ${field(msg.status.lastSync, info.last_sync)}
        </div>
        <h2 class="status-files-heading">${html(msg.status.filesHeading, {count: info.files.length})}</h2>
        <div class="status-file-list">${fileRows || `<div class="status-empty">${html(msg.common.noManagedFiles)}</div>`}</div>
    `;
}

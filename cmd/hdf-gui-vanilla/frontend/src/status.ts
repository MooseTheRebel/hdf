import { cli } from '../wailsjs/go/models';

function escapeHtml(s: string): string {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

export function renderStatus(info: cli.StatusInfo): string {
    const fileRows = info.files.map(f => `
        <div class="status-file-row">
            <span class="status-file-path">${escapeHtml(f.path)}</span>
            <span class="status-file-state">${escapeHtml(f.status)}</span>
        </div>
    `).join('');

    return `
        <div class="status-summary">
            <div class="status-field"><span class="status-label">Git push target</span><span class="status-value">${escapeHtml(info.git_push_target)}</span></div>
            <div class="status-field"><span class="status-label">Local dotfiles dir</span><span class="status-value">${escapeHtml(info.local_dotfiles_dir)}</span></div>
            <div class="status-field"><span class="status-label">Branch</span><span class="status-value">${escapeHtml(info.branch)}</span></div>
            <div class="status-field"><span class="status-label">Last commit</span><span class="status-value">${escapeHtml(info.last_commit)}</span></div>
            <div class="status-field"><span class="status-label">Last sync</span><span class="status-value">${escapeHtml(info.last_sync)}</span></div>
        </div>
        <h2 class="status-files-heading">Managed files (${info.files.length})</h2>
        <div class="status-file-list">${fileRows || '<div class="status-empty">No managed files.</div>'}</div>
    `;
}

import { cli } from '../wailsjs/go/models';

function escapeHtml(s: string): string {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

function escapeAttr(s: string): string {
    return escapeHtml(s).replace(/"/g, '&quot;');
}

export function renderLocalForm(defaultRepoPath: string): string {
    return `
        <label class="init-field-label" for="init-repo-path">Local repo path</label>
        <div class="init-field-row">
            <input type="text" id="init-repo-path" class="init-text-input" value="${escapeAttr(defaultRepoPath)}">
            <button id="init-repo-browse-btn" class="control-btn">Browse...</button>
        </div>
        <label class="init-field-label" for="init-push-target">Push target path or remote URL (optional)</label>
        <div class="init-field-row">
            <input type="text" id="init-push-target" class="init-text-input" value="">
            <button id="init-push-browse-btn" class="control-btn">Browse...</button>
        </div>
    `;
}

export function renderRemoteForm(defaultCloneDir: string): string {
    return `
        <label class="init-field-label" for="init-git-url">Remote repository URL</label>
        <div class="init-field-row">
            <input type="text" id="init-git-url" class="init-text-input" value="">
        </div>
        <label class="init-field-label" for="init-clone-dir">Clone destination</label>
        <div class="init-field-row">
            <input type="text" id="init-clone-dir" class="init-text-input" value="${escapeAttr(defaultCloneDir)}">
            <button id="init-clone-browse-btn" class="control-btn">Browse...</button>
        </div>
    `;
}

export function renderBranchCollision(branch: string): string {
    return `
        <p>A branch named <strong>${escapeHtml(branch)}</strong> already exists on the remote.</p>
        <p>Is this machine re-initializing (reuse it), or is this a different machine that happens to share this name (create a unique branch)?</p>
    `;
}

export function renderInitResult(result: cli.InitResult): string {
    return `<p class="init-result-message">${escapeHtml(result.message)}</p>`;
}

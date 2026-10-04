import { cli } from '../wailsjs/go/models';
import { msg } from '@locales';
import { escapeAttr, escapeHtml, html } from './i18n';

export function renderLocalForm(defaultRepoPath: string): string {
    return `
        <label class="init-field-label" for="init-repo-path">${html(msg.init.repoPath)}</label>
        <div class="init-field-row">
            <input type="text" id="init-repo-path" class="init-text-input" value="${escapeAttr(defaultRepoPath)}">
            <button id="init-repo-browse-btn" class="control-btn">${html(msg.common.browse)}</button>
        </div>
        <label class="init-field-label" for="init-push-target">${html(msg.init.pushTarget)}</label>
        <div class="init-field-row">
            <input type="text" id="init-push-target" class="init-text-input" value="">
            <button id="init-push-browse-btn" class="control-btn">${html(msg.common.browse)}</button>
        </div>
    `;
}

export function renderRemoteForm(defaultCloneDir: string): string {
    return `
        <label class="init-field-label" for="init-git-url">${html(msg.init.gitUrl)}</label>
        <div class="init-field-row">
            <input type="text" id="init-git-url" class="init-text-input" value="">
        </div>
        <label class="init-field-label" for="init-clone-dir">${html(msg.init.cloneDir)}</label>
        <div class="init-field-row">
            <input type="text" id="init-clone-dir" class="init-text-input" value="${escapeAttr(defaultCloneDir)}">
            <button id="init-clone-browse-btn" class="control-btn">${html(msg.common.browse)}</button>
        </div>
    `;
}

export function renderBranchCollision(branch: string): string {
    return `
        <p>${html(msg.init.collision, {}, {branch: `<strong>${escapeHtml(branch)}</strong>`})}</p>
        <p>${html(msg.init.collisionQuestion)}</p>
    `;
}

export function renderInitResult(result: cli.InitResult): string {
    return `<p class="init-result-message">${escapeHtml(result.message)}</p>`;
}

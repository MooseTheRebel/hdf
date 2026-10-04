import { msg } from '@locales';
import { renderDaemonStatus } from './daemonstatus';
import { html } from './i18n';

export function renderDaemonManagement(status: string): string {
    return `
        <div class="daemon-management-status">${renderDaemonStatus(status)}</div>
        <div class="daemon-management-actions">
            <button id="daemon-install-btn" class="control-btn">${html(msg.daemon.install)}</button>
            <button id="daemon-uninstall-btn" class="control-btn">${html(msg.daemon.uninstall)}</button>
            <button id="daemon-start-btn" class="control-btn">${html(msg.daemon.start)}</button>
            <button id="daemon-stop-btn" class="control-btn">${html(msg.daemon.stop)}</button>
        </div>
    `;
}

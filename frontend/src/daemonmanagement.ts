import { renderDaemonStatus } from './daemonstatus';

export function renderDaemonManagement(status: string): string {
    return `
        <div class="daemon-management-status">${renderDaemonStatus(status)}</div>
        <div class="daemon-management-actions">
            <button id="daemon-install-btn" class="control-btn">Install</button>
            <button id="daemon-uninstall-btn" class="control-btn">Uninstall</button>
            <button id="daemon-start-btn" class="control-btn">Start</button>
            <button id="daemon-stop-btn" class="control-btn">Stop</button>
        </div>
    `;
}

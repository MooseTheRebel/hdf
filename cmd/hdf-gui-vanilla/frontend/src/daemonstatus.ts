import { escapeHtml } from './i18n';

function badgeModifier(status: string): string {
    switch (status) {
        case 'running':
            return 'daemon-running';
        case 'stopped':
            return 'daemon-stopped';
        case 'not installed':
            return 'daemon-not-installed';
        default:
            return 'daemon-unknown';
    }
}

export function renderDaemonStatus(status: string): string {
    const modifier = badgeModifier(status);
    return `<span class="home-badge ${modifier}">${escapeHtml(status)}</span>`;
}

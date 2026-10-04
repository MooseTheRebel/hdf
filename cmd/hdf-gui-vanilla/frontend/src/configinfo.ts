import { cli } from '../wailsjs/go/models';

function escapeHtml(s: string): string {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

export function renderConfig(info: cli.ConfigInfo): string {
    if (!info.exists) {
        return `
            <p class="config-missing">No config found. Run <code>hdf init</code> to get started.</p>
        `;
    }
    return `
        <div class="config-field"><span class="config-label">Config file</span><span class="config-value">${escapeHtml(info.path)}</span></div>
        <pre class="config-content">${escapeHtml(info.content)}</pre>
    `;
}

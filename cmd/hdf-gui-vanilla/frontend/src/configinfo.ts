import { cli } from '../wailsjs/go/models';
import { msg } from '@locales';
import { escapeHtml, html } from './i18n';

export function renderConfig(info: cli.ConfigInfo): string {
    if (!info.exists) {
        return `
            <p class="config-missing">${html(msg.config.missing, {}, {command: `<code>${html(msg.config.missingCommand)}</code>`})}</p>
        `;
    }
    return `
        <div class="config-field"><span class="config-label">${html(msg.config.file)}</span><span class="config-value">${escapeHtml(info.path)}</span></div>
        <pre class="config-content">${escapeHtml(info.content)}</pre>
    `;
}

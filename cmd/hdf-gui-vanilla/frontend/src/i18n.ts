import {fmt} from '@locales';

export function escapeHtml(s: string): string {
    return s
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

// escapeAttr escapes s for a double-quoted attribute value.
export function escapeAttr(s: string): string {
    return escapeHtml(s).replace(/"/g, '&quot;');
}

// html renders a message (from @locales) for an HTML template: the message
// and its text params are escaped; markup params — HTML the caller built,
// e.g. `<code>${escapeHtml(path)}</code>` — are inserted as-is.
export function html(
    template: string,
    text: Record<string, string | number> = {},
    markup: Record<string, string> = {},
): string {
    const params: Record<string, string> = {...markup};
    for (const [name, value] of Object.entries(text)) {
        params[name] = escapeHtml(String(value));
    }
    return fmt(escapeHtml(template), params);
}

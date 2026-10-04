// The GUIs' user-visible text: the single source of truth both GUI
// frontends (cmd/hdf-gui-*/frontend) import as "@locales". Strings live in
// <locale>.json files grouped by screen, with {name} placeholders filled by
// fmt. They're plain text — never HTML — so each GUI escapes them like any
// other text and renders any markup (e.g. <code>) around them itself.
import en from './en.json';

// Messages is the shape every locale file must match; a key that's
// missing or misspelled at a call site is a type error.
export type Messages = typeof en;

// msg is the active locale's messages. Today that's always English; i18n
// means choosing among locale files here.
export const msg: Messages = en;

// fmt fills a message's {name} placeholders from params, leaving unknown
// placeholders untouched. The result is plain text.
export function fmt(template: string, params: Record<string, string | number>): string {
    return template.replace(/\{(\w+)\}/g, (placeholder, name: string) =>
        name in params ? String(params[name]) : placeholder);
}

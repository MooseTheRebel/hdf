import {describe, it, expect} from 'vitest';
import {h, type Slots} from 'vue';
import {mount} from '@vue/test-utils';
import I18nText from './I18nText';

// render mounts I18nText inside a <p> and returns that element, so
// assertions see the exact DOM (test-utils trims fragment text nodes).
function render(template: string, slots: Record<string, () => ReturnType<typeof h>> = {}): HTMLElement {
    return mount({render: () => h('p', [h(I18nText, {template}, slots as Slots)])}).element as HTMLElement;
}

describe('I18nText', () => {
    it('fills placeholders from named slots', () => {
        const p = render('Run {command} to get started.', {command: () => h('code', 'hdf init')});
        expect(p.innerHTML).toBe('Run <code>hdf init</code> to get started.');
    });

    it('escapes the message text', () => {
        expect(render('hdf enroll <path>').innerHTML).toBe('hdf enroll &lt;path&gt;');
    });

    it('leaves placeholders without a slot untouched', () => {
        expect(render('Hello {name}').textContent).toBe('Hello {name}');
    });
});

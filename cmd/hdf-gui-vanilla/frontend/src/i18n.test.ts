import {describe, it, expect} from 'vitest';
import {fmt, msg} from '@locales';
import {html} from './i18n';

describe('fmt', () => {
    it('fills placeholders', () => {
        expect(fmt('File {index} of {total}', {index: 1, total: 3})).toBe('File 1 of 3');
    });

    it('leaves unknown placeholders untouched', () => {
        expect(fmt('Hello {name}', {})).toBe('Hello {name}');
    });

    it('does not re-expand placeholders inside params', () => {
        expect(fmt('{a}', {a: '{b}', b: 'x'})).toBe('{b}');
    });
});

describe('html', () => {
    it('escapes the message itself', () => {
        expect(html('hdf enroll <path>')).toBe('hdf enroll &lt;path&gt;');
    });

    it('escapes text params', () => {
        expect(html('Error: {error}', {error: '<script>'})).toBe('Error: &lt;script&gt;');
    });

    it('inserts markup params as-is', () => {
        expect(html('Run {command}.', {}, {command: '<code>hdf init</code>'})).toBe('Run <code>hdf init</code>.');
    });
});

describe('msg', () => {
    it('is the English locale', () => {
        expect(msg.home.commands.status.command).toBe('hdf status');
    });
});

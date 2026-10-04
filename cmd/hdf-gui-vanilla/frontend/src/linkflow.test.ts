import { describe, it, expect } from 'vitest';
import { cli } from '../wailsjs/go/models';
import { renderPendingWarnings, renderIncomingFileReview, renderLinkResults } from './linkflow';

describe('renderPendingWarnings', () => {
    it('renders each warning', () => {
        const html = renderPendingWarnings(['daemon crashed', 'sync stalled']);
        expect(html).toContain('daemon crashed');
        expect(html).toContain('sync stalled');
    });

    it('escapes HTML in warning text', () => {
        const html = renderPendingWarnings(['<script>alert(1)</script>']);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

describe('renderIncomingFileReview', () => {
    it('renders the file path and a counter', () => {
        const file = new cli.IncomingFile({ path: '~/.bashrc', diff: '@@ -1 +1 @@\n-old\n+new' });
        const html = renderIncomingFileReview(file, 0, 3);
        expect(html).toContain('~/.bashrc');
        expect(html).toContain('1 of 3');
    });

    it('renders the diff body', () => {
        const file = new cli.IncomingFile({ path: '~/.bashrc', diff: '@@ -1 +1 @@\n-old\n+new' });
        const html = renderIncomingFileReview(file, 0, 1);
        expect(html).toContain('diff-hunk');
        expect(html).toContain('diff-addition');
        expect(html).toContain('diff-deletion');
    });

    it('escapes HTML in the file path', () => {
        const file = new cli.IncomingFile({ path: '<script>alert(1)</script>', diff: '' });
        const html = renderIncomingFileReview(file, 0, 1);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

describe('renderLinkResults', () => {
    it('renders a leading message when present', () => {
        const html = renderLinkResults('Already up to date.', []);
        expect(html).toContain('Already up to date.');
    });

    it('renders no message section when message is empty', () => {
        const html = renderLinkResults('', [new cli.LinkedFile({ path: '~/.bashrc', error: '' })]);
        expect(html).not.toContain('link-results-message');
    });

    it('distinguishes a successful link from a per-file error', () => {
        const results = [
            new cli.LinkedFile({ path: '~/.bashrc', error: '' }),
            new cli.LinkedFile({ path: '~/.zshrc', error: 'permission denied' }),
        ];
        const html = renderLinkResults('', results);
        expect(html).toContain('link-result-ok');
        expect(html).toContain('link-result-error');
        expect(html).toContain('permission denied');
    });

    it('escapes HTML in path and error text', () => {
        const results = [new cli.LinkedFile({ path: '<b>x</b>', error: '<script>alert(1)</script>' })];
        const html = renderLinkResults('', results);
        expect(html).not.toContain('<script>');
        expect(html).not.toContain('<b>x</b>');
        expect(html).toContain('&lt;script&gt;');
    });
});

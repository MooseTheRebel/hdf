import { describe, it, expect } from 'vitest';
import { cli } from '../wailsjs/go/models';
import { renderEnrollPreview, renderEnrollResult } from './enrollflow';

describe('renderEnrollPreview', () => {
    it('renders a new-file indicator when there is no committed version', () => {
        const info = new cli.EnrollStartInfo({ path: '~/.bashrc', isNewFile: true, diff: '' });
        const html = renderEnrollPreview(info);
        expect(html).toContain('~/.bashrc');
        expect(html).toContain('new file');
    });

    it('renders the diff body for a modified file', () => {
        const info = new cli.EnrollStartInfo({ path: '~/.bashrc', isNewFile: false, diff: '@@ -1 +1 @@\n-old\n+new' });
        const html = renderEnrollPreview(info);
        expect(html).toContain('diff-hunk');
        expect(html).toContain('diff-addition');
        expect(html).toContain('diff-deletion');
    });

    it('renders a no-changes indicator when not new and diff is empty', () => {
        const info = new cli.EnrollStartInfo({ path: '~/.bashrc', isNewFile: false, diff: '' });
        const html = renderEnrollPreview(info);
        expect(html).toContain('no changes');
    });

    it('escapes HTML in the path', () => {
        const info = new cli.EnrollStartInfo({ path: '<script>alert(1)</script>', isNewFile: true, diff: '' });
        const html = renderEnrollPreview(info);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

describe('renderEnrollResult', () => {
    it('renders the result message', () => {
        const result = new cli.EnrollResult({ message: 'Enrolled ~/.bashrc (commit abc12345)' });
        const html = renderEnrollResult(result);
        expect(html).toContain('Enrolled ~/.bashrc (commit abc12345)');
    });

    it('escapes HTML in the message', () => {
        const result = new cli.EnrollResult({ message: '<script>alert(1)</script>' });
        const html = renderEnrollResult(result);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

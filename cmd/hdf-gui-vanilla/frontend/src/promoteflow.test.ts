import { describe, it, expect } from 'vitest';
import { cli } from '../wailsjs/go/models';
import { renderPreservedFiles, renderDivergedFileReview, renderPromoteResult } from './promoteflow';

describe('renderPreservedFiles', () => {
    it('lists each preserved file path', () => {
        const files = [new cli.PreservedFile({ path: '~/.bashrc' }), new cli.PreservedFile({ path: '~/.vimrc' })];
        const html = renderPreservedFiles(files);
        expect(html).toContain('~/.bashrc');
        expect(html).toContain('~/.vimrc');
    });

    it('escapes HTML in file paths', () => {
        const files = [new cli.PreservedFile({ path: '<script>alert(1)</script>' })];
        const html = renderPreservedFiles(files);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

describe('renderDivergedFileReview', () => {
    it('renders the file path and a counter', () => {
        const file = new cli.DivergedFile({ path: '~/.bashrc', diff: '@@ -1 +1 @@\n-old\n+new' });
        const html = renderDivergedFileReview(file, 0, 2);
        expect(html).toContain('~/.bashrc');
        expect(html).toContain('1 of 2');
    });

    it('renders the diff body', () => {
        const file = new cli.DivergedFile({ path: '~/.bashrc', diff: '@@ -1 +1 @@\n-old\n+new' });
        const html = renderDivergedFileReview(file, 0, 1);
        expect(html).toContain('diff-hunk');
        expect(html).toContain('diff-addition');
        expect(html).toContain('diff-deletion');
    });

    it('escapes HTML in the file path', () => {
        const file = new cli.DivergedFile({ path: '<script>alert(1)</script>', diff: '' });
        const html = renderDivergedFileReview(file, 0, 1);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

describe('renderPromoteResult', () => {
    it('renders the result message', () => {
        const result = new cli.PromoteResult({ message: 'Promoted machine → main and pushed to origin.' });
        const html = renderPromoteResult(result);
        expect(html).toContain('Promoted machine → main and pushed to origin.');
    });

    it('escapes HTML in the message', () => {
        const result = new cli.PromoteResult({ message: '<script>alert(1)</script>' });
        const html = renderPromoteResult(result);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

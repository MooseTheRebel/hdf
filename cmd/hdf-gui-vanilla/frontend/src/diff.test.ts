import { describe, it, expect } from 'vitest';
import { renderDiffContent } from './diff';

describe('renderDiffContent', () => {
    it('wraps each line in a diff-line div', () => {
        const html = renderDiffContent('hello');
        expect(html).toContain('class="diff-line"');
        expect(html).toContain('>hello<');
    });

    it('marks added lines', () => {
        const html = renderDiffContent('+added');
        expect(html).toContain('diff-addition');
        expect(html).not.toContain('diff-deletion');
    });

    it('marks removed lines', () => {
        const html = renderDiffContent('-removed');
        expect(html).toContain('diff-deletion');
        expect(html).not.toContain('diff-addition');
    });

    it('marks hunk headers', () => {
        const html = renderDiffContent('@@ -1,3 +1,4 @@');
        expect(html).toContain('diff-hunk');
    });

    it('marks diff file header lines', () => {
        expect(renderDiffContent('diff --git a/foo b/foo')).toContain('diff-header');
        expect(renderDiffContent('index abc123..def456 100644')).toContain('diff-header');
        expect(renderDiffContent('--- a/foo')).toContain('diff-header');
        expect(renderDiffContent('+++ b/foo')).toContain('diff-header');
    });

    it('escapes HTML in line content', () => {
        const html = renderDiffContent('+<b>foo & bar</b>');
        expect(html).not.toContain('<b>');
        expect(html).toContain('&lt;b&gt;');
        expect(html).toContain('&amp;');
    });

    it('renders empty lines as a non-breaking space placeholder', () => {
        const html = renderDiffContent('');
        expect(html).toContain('> <');
    });

    it('produces one div per line', () => {
        const html = renderDiffContent('+a\n-b\n c');
        expect(html.match(/<div/g)?.length).toBe(3);
    });

});

import {describe, it, expect} from 'vitest';
import {mount} from '@vue/test-utils';
import DiffContent from './DiffContent.vue';

function lineClasses(content: string): string[] {
    return mount(DiffContent, {props: {content}}).findAll('.diff-line').map((l) => l.classes().join(' '));
}

describe('DiffContent', () => {
    it('classifies header, hunk, addition, deletion and context lines', () => {
        expect(lineClasses('diff --git a b\n@@ -1 +1 @@\n+new\n-old\n same')).toEqual([
            'diff-line diff-header',
            'diff-line diff-hunk',
            'diff-line diff-addition',
            'diff-line diff-deletion',
            'diff-line',
        ]);
    });

    it('drops one trailing newline rather than rendering an empty last line', () => {
        expect(lineClasses('+a\n')).toHaveLength(1);
    });

    it('escapes HTML in diff lines', () => {
        const wrapper = mount(DiffContent, {props: {content: '+<script>alert(1)</script>'}});
        expect(wrapper.find('script').exists()).toBe(false);
        expect(wrapper.text()).toContain('<script>alert(1)</script>');
    });
});

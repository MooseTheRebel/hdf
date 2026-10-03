import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises, type VueWrapper} from '@vue/test-utils';
import {ConfirmEnroll, GetPendingWarnings, StartEnroll} from '../../wailsjs/go/cli/App';
import {cli} from '../../wailsjs/go/models';
import EnrollView from './EnrollView.vue';

vi.mock('../../wailsjs/go/cli/App');

function button(wrapper: VueWrapper, label: string) {
    const b = wrapper.findAll('button').find((x) => x.text() === label);
    if (!b) throw new Error(`no ${label} button`);
    return b;
}

describe('EnrollView', () => {
    beforeEach(() => {
        vi.resetAllMocks();
        vi.mocked(GetPendingWarnings).mockResolvedValue([]);
    });

    it('previews the diff, then enrolls on confirm', async () => {
        vi.mocked(StartEnroll).mockResolvedValue(new cli.EnrollStartInfo({path: '~/.bashrc', isNewFile: false, diff: '@@ -1 +1 @@\n+new'}));
        vi.mocked(ConfirmEnroll).mockResolvedValue(new cli.EnrollResult({message: 'Enrolled ~/.bashrc'}));
        const wrapper = mount(EnrollView, {props: {path: '/home/u/.bashrc'}});
        await flushPromises();
        expect(StartEnroll).toHaveBeenCalledWith('/home/u/.bashrc');
        expect(wrapper.find('.enroll-preview-path').text()).toBe('~/.bashrc');
        expect(wrapper.find('.diff-addition').exists()).toBe(true);

        await button(wrapper, 'Enroll').trigger('click');
        await flushPromises();
        expect(wrapper.find('.enroll-result-message').text()).toBe('Enrolled ~/.bashrc');
    });

    it('notes a new file instead of a diff', async () => {
        vi.mocked(StartEnroll).mockResolvedValue(new cli.EnrollStartInfo({path: '~/.new', isNewFile: true, diff: ''}));
        const wrapper = mount(EnrollView, {props: {path: '/x'}});
        await flushPromises();
        expect(wrapper.find('.enroll-preview-note').text()).toBe('This is a new file.');
    });

    it('notes when there are no changes', async () => {
        vi.mocked(StartEnroll).mockResolvedValue(new cli.EnrollStartInfo({path: '~/.same', isNewFile: false, diff: ''}));
        const wrapper = mount(EnrollView, {props: {path: '/x'}});
        await flushPromises();
        expect(wrapper.find('.enroll-preview-note').text()).toBe('There are no changes to enroll.');
    });

    it('shows pending warnings first, escaped, and continues on request', async () => {
        vi.mocked(GetPendingWarnings).mockResolvedValue(['<b>daemon crashed</b>']);
        vi.mocked(StartEnroll).mockResolvedValue(new cli.EnrollStartInfo({path: '~/.x', isNewFile: true, diff: ''}));
        const wrapper = mount(EnrollView, {props: {path: '/x'}});
        await flushPromises();
        expect(wrapper.find('b').exists()).toBe(false);
        expect(wrapper.find('.link-warning-row').text()).toBe('<b>daemon crashed</b>');
        expect(StartEnroll).not.toHaveBeenCalled();

        await button(wrapper, 'Continue').trigger('click');
        await flushPromises();
        expect(StartEnroll).toHaveBeenCalled();
    });

    it('shows a file-picker error without calling the backend', async () => {
        const wrapper = mount(EnrollView, {props: {path: '', pickError: 'Error picking a file: boom'}});
        await flushPromises();
        expect(wrapper.text()).toContain('Error picking a file: boom');
        expect(GetPendingWarnings).not.toHaveBeenCalled();
    });
});

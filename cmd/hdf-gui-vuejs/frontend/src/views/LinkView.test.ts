import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises} from '@vue/test-utils';
import {AcceptIncomingFile, FinishLink, GetPendingWarnings, StartLink} from '../../wailsjs/go/cli/App';
import {cli} from '../../wailsjs/go/models';
import LinkView from './LinkView.vue';

vi.mock('../../wailsjs/go/cli/App');

const files = [
    new cli.IncomingFile({path: '~/.bashrc', diff: '@@ -1 +1 @@\n-old\n+new'}),
    new cli.IncomingFile({path: '~/.vimrc', diff: '+set number'}),
];

describe('LinkView', () => {
    beforeEach(() => {
        vi.resetAllMocks();
        vi.mocked(GetPendingWarnings).mockResolvedValue([]);
    });

    it('reviews each incoming file, then shows link results', async () => {
        vi.mocked(StartLink).mockResolvedValue(new cli.LinkStartInfo({message: '', incomingFiles: files}));
        vi.mocked(AcceptIncomingFile).mockResolvedValue();
        vi.mocked(FinishLink).mockResolvedValue([
            new cli.LinkedFile({path: '~/.bashrc', error: ''}),
            new cli.LinkedFile({path: '~/.vimrc', error: 'permission denied'}),
        ]);
        const wrapper = mount(LinkView, {props: {noFetch: true}});
        await flushPromises();
        expect(StartLink).toHaveBeenCalledWith(true);
        expect(wrapper.find('.link-review-counter').text()).toBe('File 1 of 2');
        expect(wrapper.find('.link-review-path').text()).toBe('~/.bashrc');

        await wrapper.find('#link-accept-btn').trigger('click');
        await flushPromises();
        expect(AcceptIncomingFile).toHaveBeenCalledWith(0);
        expect(wrapper.find('.link-review-counter').text()).toBe('File 2 of 2');

        await wrapper.find('#link-skip-btn').trigger('click');
        await flushPromises();
        expect(AcceptIncomingFile).toHaveBeenCalledTimes(1);
        const rows = wrapper.findAll('.link-result-row');
        expect(rows[0].classes()).toContain('link-result-ok');
        expect(rows[0].find('.link-result-path').text()).toBe('~/.bashrc');
        expect(rows[0].find('.link-result-status').text()).toBe('linked');
        expect(rows[1].classes()).toContain('link-result-error');
        expect(rows[1].find('.link-result-path').text()).toBe('~/.vimrc');
        expect(rows[1].find('.link-result-status').text()).toBe('permission denied');
    });

    it('goes straight to results, with the start message, when nothing is incoming', async () => {
        vi.mocked(StartLink).mockResolvedValue(new cli.LinkStartInfo({message: 'Already up to date.', incomingFiles: []}));
        vi.mocked(FinishLink).mockResolvedValue([]);
        const wrapper = mount(LinkView, {props: {noFetch: false}});
        await flushPromises();
        expect(wrapper.find('.link-results-message').text()).toBe('Already up to date.');
        expect(wrapper.find('.link-results-empty').exists()).toBe(true);
    });

    it('offers to skip a file that fails to accept', async () => {
        vi.mocked(StartLink).mockResolvedValue(new cli.LinkStartInfo({message: '', incomingFiles: files}));
        vi.mocked(AcceptIncomingFile).mockRejectedValue('conflict');
        const wrapper = mount(LinkView, {props: {noFetch: false}});
        await flushPromises();
        await wrapper.find('#link-accept-btn').trigger('click');
        await flushPromises();
        expect(wrapper.find('.link-error').text()).toBe('Error accepting ~/.bashrc: conflict');

        await wrapper.findAll('button').find((b) => b.text() === 'Skip and continue')!.trigger('click');
        expect(wrapper.find('.link-review-counter').text()).toBe('File 2 of 2');
    });

    it('escapes HTML in incoming file paths', async () => {
        vi.mocked(StartLink).mockResolvedValue(new cli.LinkStartInfo({message: '', incomingFiles: [new cli.IncomingFile({path: '<img src=x>', diff: ''})]}));
        const wrapper = mount(LinkView, {props: {noFetch: false}});
        await flushPromises();
        expect(wrapper.find('img').exists()).toBe(false);
        expect(wrapper.find('.link-review-path').text()).toBe('<img src=x>');
    });
});

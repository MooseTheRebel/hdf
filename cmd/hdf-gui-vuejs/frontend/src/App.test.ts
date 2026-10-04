import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises} from '@vue/test-utils';
import {GetCurrentIndex, GetDiffContent, GetStatus, GetTotalDiffs, HasDiff, IsInitialized} from '../wailsjs/go/cli/App';
import {cli} from '../wailsjs/go/models';
import App from './App.vue';

vi.mock('../wailsjs/go/cli/App');

describe('App', () => {
    beforeEach(() => {
        vi.resetAllMocks();
        vi.mocked(IsInitialized).mockResolvedValue(true);
    });

    it('opens the diff viewer when diffs are queued', async () => {
        vi.mocked(HasDiff).mockResolvedValue(true);
        vi.mocked(GetDiffContent).mockResolvedValue('+a');
        vi.mocked(GetCurrentIndex).mockResolvedValue(0);
        vi.mocked(GetTotalDiffs).mockResolvedValue(2);
        const wrapper = mount(App);
        await flushPromises();
        expect(wrapper.find('.diff-counter').text()).toBe('Diff 1 of 2');
    });

    it('opens home otherwise, even if HasDiff fails', async () => {
        vi.mocked(HasDiff).mockRejectedValue('no runtime');
        const wrapper = mount(App);
        await flushPromises();
        expect(wrapper.find('.home-container').exists()).toBe(true);
    });

    it('navigates from home to a view and back', async () => {
        vi.mocked(HasDiff).mockResolvedValue(false);
        vi.mocked(GetStatus).mockResolvedValue(new cli.StatusInfo({files: []}));
        const wrapper = mount(App);
        await flushPromises();
        await wrapper.find('#status-btn').trigger('click');
        await flushPromises();
        expect(wrapper.find('.status-container').exists()).toBe(true);

        await wrapper.find('.status-controls button').trigger('click');
        await flushPromises();
        expect(wrapper.find('.home-container').exists()).toBe(true);
    });
});

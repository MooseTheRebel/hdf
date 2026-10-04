import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises} from '@vue/test-utils';
import {IsInitialized, PickFileToEnroll} from '../../wailsjs/go/cli/App';
import HomeView from './HomeView.vue';

vi.mock('../../wailsjs/go/cli/App');

async function mountWith(initialized: boolean) {
    vi.mocked(IsInitialized).mockResolvedValue(initialized);
    const wrapper = mount(HomeView);
    await flushPromises();
    return wrapper;
}

describe('HomeView', () => {
    beforeEach(() => vi.resetAllMocks());

    it('shows the command list when initialized', async () => {
        const wrapper = await mountWith(true);
        expect(wrapper.find('.initialized').exists()).toBe(true);
        expect(wrapper.find('#status-btn').exists()).toBe(true);
    });

    it('shows Get Started when not initialized', async () => {
        const wrapper = await mountWith(false);
        await wrapper.find('#get-started-btn').trigger('click');
        expect(wrapper.emitted('navigate')).toEqual([[{name: 'init'}]]);
    });

    it('shows a config error, escaped', async () => {
        vi.mocked(IsInitialized).mockRejectedValue('<b>corrupt</b>');
        const wrapper = mount(HomeView);
        await flushPromises();
        expect(wrapper.find('b').exists()).toBe(false);
        expect(wrapper.find('#error-message').text()).toBe('<b>corrupt</b>');
    });

    it('navigates to enroll with the picked file', async () => {
        vi.mocked(PickFileToEnroll).mockResolvedValue('/home/u/.bashrc');
        const wrapper = await mountWith(true);
        await wrapper.find('#enroll-btn').trigger('click');
        await flushPromises();
        expect(wrapper.emitted('navigate')).toEqual([[{name: 'enroll', path: '/home/u/.bashrc'}]]);
    });

    it('stays put when the file picker is cancelled', async () => {
        vi.mocked(PickFileToEnroll).mockResolvedValue('');
        const wrapper = await mountWith(true);
        await wrapper.find('#enroll-btn').trigger('click');
        await flushPromises();
        expect(wrapper.emitted('navigate')).toBeUndefined();
    });

    it('navigates to link with or without fetch', async () => {
        const wrapper = await mountWith(true);
        await wrapper.find('#link-btn').trigger('click');
        await wrapper.find('#link-no-fetch-btn').trigger('click');
        expect(wrapper.emitted('navigate')).toEqual([[{name: 'link', noFetch: false}], [{name: 'link', noFetch: true}]]);
    });
});

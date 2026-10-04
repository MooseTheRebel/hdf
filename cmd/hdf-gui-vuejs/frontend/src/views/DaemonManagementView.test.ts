import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises} from '@vue/test-utils';
import {GetDaemonStatus, InstallDaemon, StopDaemon} from '../../wailsjs/go/cli/App';
import DaemonManagementView from './DaemonManagementView.vue';

vi.mock('../../wailsjs/go/cli/App');

describe('DaemonManagementView', () => {
    beforeEach(() => vi.resetAllMocks());

    it('runs an action, reports success, and refreshes the status', async () => {
        vi.mocked(GetDaemonStatus).mockResolvedValueOnce('not installed').mockResolvedValueOnce('running');
        vi.mocked(InstallDaemon).mockResolvedValue();
        const wrapper = mount(DaemonManagementView);
        await flushPromises();
        expect(wrapper.find('.home-badge').text()).toBe('not installed');

        await wrapper.find('#daemon-install-btn').trigger('click');
        await flushPromises();
        expect(InstallDaemon).toHaveBeenCalledOnce();
        expect(wrapper.find('#daemon-management-result').text()).toBe('Daemon installed and started.');
        expect(wrapper.find('.home-badge').text()).toBe('running');
    });

    it('reports an action error', async () => {
        vi.mocked(GetDaemonStatus).mockResolvedValue('running');
        vi.mocked(StopDaemon).mockRejectedValue('permission denied');
        const wrapper = mount(DaemonManagementView);
        await flushPromises();
        await wrapper.find('#daemon-stop-btn').trigger('click');
        await flushPromises();
        expect(wrapper.find('#daemon-management-result').text()).toBe('Error: permission denied');
    });
});

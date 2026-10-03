import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises} from '@vue/test-utils';
import {GetStatus} from '../../wailsjs/go/cli/App';
import {cli} from '../../wailsjs/go/models';
import StatusView from './StatusView.vue';

vi.mock('../../wailsjs/go/cli/App');

function statusInfo(overrides: Partial<cli.StatusInfo> = {}): cli.StatusInfo {
    return new cli.StatusInfo({
        git_push_target: 'git@example.com:user/dotfiles.git',
        local_dotfiles_dir: '/home/user/dotfiles',
        branch: 'host-laptop',
        last_commit: 'abc123',
        last_sync: '2026-07-26 10:30:00',
        files: [],
        ...overrides,
    });
}

async function mountWith(info: cli.StatusInfo) {
    vi.mocked(GetStatus).mockResolvedValue(info);
    const wrapper = mount(StatusView);
    await flushPromises();
    return wrapper;
}

describe('StatusView', () => {
    beforeEach(() => vi.resetAllMocks());

    it('renders the summary fields', async () => {
        const text = (await mountWith(statusInfo())).text();
        for (const v of ['git@example.com:user/dotfiles.git', '/home/user/dotfiles', 'host-laptop', 'abc123', '2026-07-26 10:30:00']) {
            expect(text).toContain(v);
        }
    });

    it('renders each managed file with its status and a count', async () => {
        const wrapper = await mountWith(statusInfo({files: [
            new cli.FileStatus({path: '~/.bashrc', status: 'ok'}),
            new cli.FileStatus({path: '~/.vimrc', status: 'CHANGED (uncommitted)'}),
        ]}));
        const rows = wrapper.findAll('.status-file-row').map((r) => [r.find('.status-file-path').text(), r.find('.status-file-state').text()]);
        expect(rows).toEqual([['~/.bashrc', 'ok'], ['~/.vimrc', 'CHANGED (uncommitted)']]);
        expect(wrapper.find('.status-files-heading').text()).toBe('Managed files (2)');
    });

    it('shows an empty state with no managed files', async () => {
        expect((await mountWith(statusInfo())).find('.status-empty').exists()).toBe(true);
    });

    it('escapes HTML in file paths', async () => {
        const wrapper = await mountWith(statusInfo({files: [new cli.FileStatus({path: '<img src=x>', status: 'ok'})]}));
        expect(wrapper.find('img').exists()).toBe(false);
        expect(wrapper.find('.status-file-path').text()).toBe('<img src=x>');
    });

    it('shows a load error', async () => {
        vi.mocked(GetStatus).mockRejectedValue('no config');
        const wrapper = mount(StatusView);
        await flushPromises();
        expect(wrapper.text()).toContain('Error loading status: no config');
    });

    it('emits back', async () => {
        const wrapper = await mountWith(statusInfo());
        await wrapper.find('button').trigger('click');
        expect(wrapper.emitted('back')).toHaveLength(1);
    });
});

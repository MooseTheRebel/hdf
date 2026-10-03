import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises, type VueWrapper} from '@vue/test-utils';
import {DefaultRepoPath, FinishInit, ResolveBranchCollision, StartInitLocal, StartInitRemote} from '../../wailsjs/go/cli/App';
import {cli} from '../../wailsjs/go/models';
import InitView from './InitView.vue';

vi.mock('../../wailsjs/go/cli/App');

function button(wrapper: VueWrapper, label: string) {
    const b = wrapper.findAll('button').find((x) => x.text() === label);
    if (!b) throw new Error(`no ${label} button`);
    return b;
}

describe('InitView', () => {
    beforeEach(() => {
        vi.resetAllMocks();
        vi.mocked(DefaultRepoPath).mockResolvedValue('/home/u/.local/share/hdf/repo');
        vi.mocked(FinishInit).mockResolvedValue(new cli.InitResult({message: 'hdf initialized (branch laptop).'}));
    });

    it('initializes locally with the default path pre-filled and edited fields passed through', async () => {
        vi.mocked(StartInitLocal).mockResolvedValue(new cli.InitStartInfo({}));
        const wrapper = mount(InitView);
        await button(wrapper, 'Local directory').trigger('click');
        await flushPromises();
        expect((wrapper.find('#init-repo-path').element as HTMLInputElement).value).toBe('/home/u/.local/share/hdf/repo');

        await wrapper.find('#init-push-target').setValue('/srv/dotfiles.git');
        await wrapper.find('#init-continue-btn').trigger('click');
        await flushPromises();
        expect(StartInitLocal).toHaveBeenCalledWith('/home/u/.local/share/hdf/repo', '/srv/dotfiles.git');
        expect(wrapper.find('.init-result-message').text()).toBe('hdf initialized (branch laptop).');
    });

    it('initializes from a remote', async () => {
        vi.mocked(StartInitRemote).mockResolvedValue(new cli.InitStartInfo({}));
        const wrapper = mount(InitView);
        await button(wrapper, 'Remote repository').trigger('click');
        await flushPromises();
        await wrapper.find('#init-git-url').setValue('git@example.com:u/dotfiles.git');
        await wrapper.find('#init-continue-btn').trigger('click');
        await flushPromises();
        expect(StartInitRemote).toHaveBeenCalledWith('git@example.com:u/dotfiles.git', '/home/u/.local/share/hdf/repo');
    });

    it('asks about a branch collision, escaped, and resolves it', async () => {
        vi.mocked(StartInitLocal).mockResolvedValue(new cli.InitStartInfo({collision: {branch: '<b>laptop</b>'}}));
        vi.mocked(ResolveBranchCollision).mockResolvedValue();
        const wrapper = mount(InitView);
        await button(wrapper, 'Local directory').trigger('click');
        await flushPromises();
        await wrapper.find('#init-continue-btn').trigger('click');
        await flushPromises();
        expect(wrapper.find('strong').text()).toBe('<b>laptop</b>');
        expect(wrapper.find('b').exists()).toBe(false);

        await button(wrapper, 'Create a unique branch').trigger('click');
        await flushPromises();
        expect(ResolveBranchCollision).toHaveBeenCalledWith(true);
        expect(FinishInit).toHaveBeenCalled();
    });
});

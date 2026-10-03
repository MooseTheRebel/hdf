import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises} from '@vue/test-utils';
import {FinishPromote, ResolveDivergedFile, StartPromote} from '../../wailsjs/go/cli/App';
import {cli} from '../../wailsjs/go/models';
import PromoteView from './PromoteView.vue';

vi.mock('../../wailsjs/go/cli/App');

describe('PromoteView', () => {
    beforeEach(() => {
        vi.resetAllMocks();
        vi.mocked(FinishPromote).mockResolvedValue(new cli.PromoteResult({message: 'Promoted to main.'}));
    });

    it('lists preserved files, then reviews diverged files, then promotes', async () => {
        vi.mocked(StartPromote).mockResolvedValue(new cli.PromoteStartInfo({
            preserved: [{path: '~/.zshrc'}],
            diverged: [{path: '~/.bashrc', diff: '+mine'}],
        }));
        vi.mocked(ResolveDivergedFile).mockResolvedValue();
        const wrapper = mount(PromoteView);
        await flushPromises();
        expect(wrapper.find('.link-warning-row').text()).toBe('~/.zshrc');

        await wrapper.findAll('button').find((b) => b.text() === 'Continue')!.trigger('click');
        expect(wrapper.find('.link-review-path').text()).toBe('~/.bashrc');

        await wrapper.find('#promote-keep-theirs-btn').trigger('click');
        await flushPromises();
        expect(ResolveDivergedFile).toHaveBeenCalledWith(0, false);
        expect(wrapper.find('.init-result-message').text()).toBe('Promoted to main.');
    });

    it('promotes directly when nothing needs review', async () => {
        vi.mocked(StartPromote).mockResolvedValue(new cli.PromoteStartInfo({preserved: [], diverged: []}));
        const wrapper = mount(PromoteView);
        await flushPromises();
        expect(FinishPromote).toHaveBeenCalled();
        expect(wrapper.find('.init-result-message').text()).toBe('Promoted to main.');
    });

    it('escapes HTML in preserved file paths', async () => {
        vi.mocked(StartPromote).mockResolvedValue(new cli.PromoteStartInfo({preserved: [{path: '<b>x</b>'}], diverged: []}));
        const wrapper = mount(PromoteView);
        await flushPromises();
        expect(wrapper.find('b').exists()).toBe(false);
        expect(wrapper.find('.link-warning-row').text()).toBe('<b>x</b>');
    });
});

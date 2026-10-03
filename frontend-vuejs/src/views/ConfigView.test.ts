import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises} from '@vue/test-utils';
import {GetConfig} from '../../wailsjs/go/cli/App';
import {cli} from '../../wailsjs/go/models';
import ConfigView from './ConfigView.vue';

vi.mock('../../wailsjs/go/cli/App');

async function mountWith(info: cli.ConfigInfo) {
    vi.mocked(GetConfig).mockResolvedValue(info);
    const wrapper = mount(ConfigView);
    await flushPromises();
    return wrapper;
}

describe('ConfigView', () => {
    beforeEach(() => vi.resetAllMocks());

    it('tells the user to run hdf init when there is no config', async () => {
        const wrapper = await mountWith(new cli.ConfigInfo({path: '/x', exists: false, content: ''}));
        expect(wrapper.find('.config-missing').text()).toContain('hdf init');
    });

    it('renders the config path and content', async () => {
        const wrapper = await mountWith(new cli.ConfigInfo({path: '/home/u/.config/hdf/config.toml', exists: true, content: 'branch = "laptop"'}));
        expect(wrapper.find('.config-value').text()).toBe('/home/u/.config/hdf/config.toml');
        expect(wrapper.find('.config-content').text()).toBe('branch = "laptop"');
    });

    it('escapes HTML in the config content', async () => {
        const wrapper = await mountWith(new cli.ConfigInfo({path: '/x', exists: true, content: '<script>x</script>'}));
        expect(wrapper.find('script').exists()).toBe(false);
        expect(wrapper.find('.config-content').text()).toBe('<script>x</script>');
    });
});

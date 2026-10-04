import { describe, it, expect } from 'vitest';
import { renderConfig } from './configinfo';
import { cli } from '../wailsjs/go/models';

describe('renderConfig', () => {
    it('renders the path and content when the config exists', () => {
        const html = renderConfig(new cli.ConfigInfo({
            path: '/home/user/.config/hdf/config.toml',
            exists: true,
            content: 'branch = "host-laptop"\n',
        }));
        expect(html).toContain('/home/user/.config/hdf/config.toml');
        expect(html).toContain('branch = "host-laptop"');
    });

    it('shows a message when the config does not exist', () => {
        const html = renderConfig(new cli.ConfigInfo({
            path: '/home/user/.config/hdf/config.toml',
            exists: false,
            content: '',
        }));
        expect(html).toContain('No config found');
        expect(html).toContain('hdf init');
    });

    it('escapes HTML in the content', () => {
        const html = renderConfig(new cli.ConfigInfo({
            path: '/home/user/.config/hdf/config.toml',
            exists: true,
            content: '<script>evil()</script>',
        }));
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });

    it('escapes HTML in the path', () => {
        const html = renderConfig(new cli.ConfigInfo({
            path: '/home/<script>/config.toml',
            exists: true,
            content: '',
        }));
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

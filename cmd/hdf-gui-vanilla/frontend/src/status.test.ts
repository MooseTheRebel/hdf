import { describe, it, expect } from 'vitest';
import { renderStatus } from './status';
import { cli } from '../wailsjs/go/models';

function makeStatusInfo(overrides: Partial<{
    git_push_target: string;
    local_dotfiles_dir: string;
    branch: string;
    last_commit: string;
    last_sync: string;
    files: { path: string; status: string }[];
}> = {}): cli.StatusInfo {
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

describe('renderStatus', () => {
    it('renders the summary fields', () => {
        const html = renderStatus(makeStatusInfo());
        expect(html).toContain('git@example.com:user/dotfiles.git');
        expect(html).toContain('/home/user/dotfiles');
        expect(html).toContain('host-laptop');
        expect(html).toContain('abc123');
        expect(html).toContain('2026-07-26 10:30:00');
    });

    it('renders each managed file with its status', () => {
        const html = renderStatus(makeStatusInfo({
            files: [
                { path: '~/.bashrc', status: 'ok' },
                { path: '~/.vimrc', status: 'CHANGED (uncommitted)' },
            ],
        }));
        expect(html).toContain('~/.bashrc');
        expect(html).toContain('ok');
        expect(html).toContain('~/.vimrc');
        expect(html).toContain('CHANGED (uncommitted)');
    });

    it('shows a message when there are no managed files', () => {
        const html = renderStatus(makeStatusInfo());
        expect(html).toContain('No managed files');
    });

    it('escapes HTML in file paths', () => {
        const html = renderStatus(makeStatusInfo({
            files: [{ path: '~/<script>evil()</script>', status: 'ok' }],
        }));
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });

    it('escapes HTML in summary fields', () => {
        const html = renderStatus(makeStatusInfo({ branch: '<b>x</b>' }));
        expect(html).not.toContain('<b>x</b>');
        expect(html).toContain('&lt;b&gt;');
    });
});

import { describe, it, expect } from 'vitest';
import { renderDaemonManagement } from './daemonmanagement';

describe('renderDaemonManagement', () => {
    it('renders the current daemon status badge', () => {
        const html = renderDaemonManagement('running');
        expect(html).toContain('home-badge daemon-running');
        expect(html).toContain('running');
    });

    it('renders install, uninstall, start, and stop buttons', () => {
        const html = renderDaemonManagement('stopped');
        expect(html).toContain('id="daemon-install-btn"');
        expect(html).toContain('id="daemon-uninstall-btn"');
        expect(html).toContain('id="daemon-start-btn"');
        expect(html).toContain('id="daemon-stop-btn"');
    });

    it('escapes HTML in the status text', () => {
        const html = renderDaemonManagement('<script>alert(1)</script>');
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

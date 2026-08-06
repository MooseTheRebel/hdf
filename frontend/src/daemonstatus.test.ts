import { describe, it, expect } from 'vitest';
import { renderDaemonStatus } from './daemonstatus';

describe('renderDaemonStatus', () => {
    it('renders a running badge for "running"', () => {
        const html = renderDaemonStatus('running');
        expect(html).toContain('home-badge daemon-running');
        expect(html).toContain('running');
    });

    it('renders a stopped badge for "stopped"', () => {
        const html = renderDaemonStatus('stopped');
        expect(html).toContain('home-badge daemon-stopped');
        expect(html).toContain('stopped');
    });

    it('renders a not-installed badge for "not installed"', () => {
        const html = renderDaemonStatus('not installed');
        expect(html).toContain('home-badge daemon-not-installed');
        expect(html).toContain('not installed');
    });

    it('renders an unknown badge for an unrecognized status', () => {
        const html = renderDaemonStatus('unknown');
        expect(html).toContain('home-badge daemon-unknown');
        expect(html).toContain('unknown');
    });

    it('renders an unknown badge for a value the server never documented', () => {
        const html = renderDaemonStatus('totally-unexpected');
        expect(html).toContain('home-badge daemon-unknown');
        expect(html).toContain('totally-unexpected');
    });

    it('escapes HTML in the status text', () => {
        const html = renderDaemonStatus('<script>alert(1)</script>');
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

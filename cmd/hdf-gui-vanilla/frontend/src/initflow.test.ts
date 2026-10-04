import { describe, it, expect } from 'vitest';
import { cli } from '../wailsjs/go/models';
import { renderLocalForm, renderRemoteForm, renderBranchCollision, renderInitResult } from './initflow';

describe('renderLocalForm', () => {
    it('pre-fills the repo path input with the default path', () => {
        const html = renderLocalForm('/home/user/.local/share/hdf/repo');
        expect(html).toContain('/home/user/.local/share/hdf/repo');
        expect(html).toContain('id="init-repo-path"');
        expect(html).toContain('id="init-push-target"');
        expect(html).toContain('id="init-repo-browse-btn"');
    });

    it('escapes HTML in the default path', () => {
        const html = renderLocalForm('<script>alert(1)</script>');
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

describe('renderRemoteForm', () => {
    it('pre-fills the clone destination input with the default path', () => {
        const html = renderRemoteForm('/home/user/.local/share/hdf/repo');
        expect(html).toContain('/home/user/.local/share/hdf/repo');
        expect(html).toContain('id="init-git-url"');
        expect(html).toContain('id="init-clone-dir"');
        expect(html).toContain('id="init-clone-browse-btn"');
    });

    it('escapes HTML in the default path', () => {
        const html = renderRemoteForm('<script>alert(1)</script>');
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

describe('renderBranchCollision', () => {
    it('mentions the colliding branch name', () => {
        const html = renderBranchCollision('shared-host');
        expect(html).toContain('shared-host');
    });

    it('escapes HTML in the branch name', () => {
        const html = renderBranchCollision('<script>alert(1)</script>');
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

describe('renderInitResult', () => {
    it('renders the result message', () => {
        const result = new cli.InitResult({ message: 'hdf initialized (branch shared-host)' });
        const html = renderInitResult(result);
        expect(html).toContain('hdf initialized (branch shared-host)');
    });

    it('escapes HTML in the message', () => {
        const result = new cli.InitResult({ message: '<script>alert(1)</script>' });
        const html = renderInitResult(result);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

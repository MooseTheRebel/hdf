import { describe, it, expect } from 'vitest';
import { cli } from '../wailsjs/go/models';
import { renderReportIssueResult } from './reportissueflow';

describe('renderReportIssueResult', () => {
    it('renders the report path', () => {
        const result = new cli.ReportIssueResult({ path: '/tmp/hdf-report-x.zip' });
        const html = renderReportIssueResult(result);
        expect(html).toContain('/tmp/hdf-report-x.zip');
    });

    it('escapes HTML in the path', () => {
        const result = new cli.ReportIssueResult({ path: '<script>alert(1)</script>' });
        const html = renderReportIssueResult(result);
        expect(html).not.toContain('<script>');
        expect(html).toContain('&lt;script&gt;');
    });
});

import {describe, it, expect, vi, beforeEach} from 'vitest';
import {mount, flushPromises} from '@vue/test-utils';
import {SubmitReportIssue} from '../../wailsjs/go/cli/App';
import {cli} from '../../wailsjs/go/models';
import ReportIssueView from './ReportIssueView.vue';

vi.mock('../../wailsjs/go/cli/App');

describe('ReportIssueView', () => {
    beforeEach(() => vi.resetAllMocks());

    it('submits the description and shows where the report was written, escaped', async () => {
        vi.mocked(SubmitReportIssue).mockResolvedValue(new cli.ReportIssueResult({path: '/tmp/<b>report</b>.zip'}));
        const wrapper = mount(ReportIssueView);
        await wrapper.find('#report-issue-text').setValue('sync stopped');
        await wrapper.find('#report-issue-submit-btn').trigger('click');
        await flushPromises();
        expect(SubmitReportIssue).toHaveBeenCalledWith('sync stopped');
        expect(wrapper.find('b').exists()).toBe(false);
        expect(wrapper.find('code').text()).toBe('/tmp/<b>report</b>.zip');
    });

    it('shows a build error', async () => {
        vi.mocked(SubmitReportIssue).mockRejectedValue('disk full');
        const wrapper = mount(ReportIssueView);
        await wrapper.find('#report-issue-submit-btn').trigger('click');
        await flushPromises();
        expect(wrapper.text()).toContain('Error building report: disk full');
    });
});

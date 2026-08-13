package cli

import (
	"errors"
	"fmt"
	"hdf/report"
)

// ReportIssueResult is returned after building a diagnostic report.
type ReportIssueResult struct {
	Path string `json:"path"`
}

// computeReportIssue builds a diagnostic report and returns the path it was
// written to, translating ErrRepoTooLarge into an actionable message — the
// GUI-oriented counterpart to runReportIssue, returning a result instead of
// printing.
func computeReportIssue(opts report.BuildOptions) (*ReportIssueResult, error) {
	path, err := buildReport(opts, version)
	if err != nil {
		if errors.Is(err, report.ErrRepoTooLarge) {
			return nil, fmt.Errorf("dotfiles repo is too large to include in a report (compressed size over %d bytes) — prune history or contact your admin another way", report.MaxRepoZipBytes)
		}
		return nil, fmt.Errorf("building report: %w", err)
	}
	return &ReportIssueResult{Path: path}, nil
}

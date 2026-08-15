package cli

import (
	"errors"
	"hdf/report"
	"strings"
	"testing"
)

func TestComputeReportIssue_Success(t *testing.T) {
	origBuild := buildReport
	defer func() { buildReport = origBuild }()
	var gotOpts report.BuildOptions
	buildReport = func(opts report.BuildOptions, version string) (string, error) {
		gotOpts = opts
		return testReportZipPath, nil
	}

	result, err := computeReportIssue(report.BuildOptions{Trigger: report.TriggerManual, UserText: testReportUserText})
	if err != nil {
		t.Fatalf("computeReportIssue: %v", err)
	}
	if result.Path != testReportZipPath {
		t.Errorf("Path = %q, want %q", result.Path, testReportZipPath)
	}
	if gotOpts.Trigger != report.TriggerManual || gotOpts.UserText != testReportUserText {
		t.Errorf("buildReport called with %+v", gotOpts)
	}
}

func TestComputeReportIssue_RepoTooLargeGivesFriendlyError(t *testing.T) {
	origBuild := buildReport
	defer func() { buildReport = origBuild }()
	buildReport = func(report.BuildOptions, string) (string, error) {
		return "", report.ErrRepoTooLarge
	}

	_, err := computeReportIssue(report.BuildOptions{})
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("computeReportIssue err = %v, want a message containing \"too large\"", err)
	}
}

func TestComputeReportIssue_OtherErrorPropagates(t *testing.T) {
	origBuild := buildReport
	defer func() { buildReport = origBuild }()
	buildReport = func(report.BuildOptions, string) (string, error) {
		return "", errors.New("boom")
	}

	_, err := computeReportIssue(report.BuildOptions{})
	if err == nil {
		t.Fatal("expected error to propagate, got nil")
	}
}

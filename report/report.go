// report/report.go
package report

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"hdf/config"
	"hdf/eventlog"
	"hdf/repo"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// configEntryName is the name the redacted config.toml is stored under
// inside the report zip.
const configEntryName = "config.toml"

// TriggerType classifies what caused a report to be built.
type TriggerType string

// Recognized TriggerType values.
const (
	TriggerManual      TriggerType = "manual"
	TriggerPanic       TriggerType = "panic"
	TriggerDaemonCrash TriggerType = "daemon_crash"
)

// BuildOptions configures Build.
type BuildOptions struct {
	CfgPath     string
	StatePath   string
	Trigger     TriggerType
	UserText    string // free-text description; empty for automatic triggers
	CrashDetail string // populated for TriggerPanic / TriggerDaemonCrash
	OutDir      string // directory the report zip is written into
}

// summary is the JSON structure written as summary.json inside the report.
type summary struct {
	Time        time.Time   `json:"time"`
	HDFVersion  string      `json:"hdf_version"`
	Trigger     TriggerType `json:"trigger"`
	UserText    string      `json:"user_text,omitempty"`
	CrashDetail string      `json:"crash_detail,omitempty"`
	Branch      string      `json:"branch"`
}

// reportContents holds every piece of data that goes into the report zip,
// gathered before any output file is created.
type reportContents struct {
	time          time.Time
	summaryJSON   []byte
	hostsJSON     []byte
	eventLogBytes []byte
	cfgBytes      []byte
	stateBytes    []byte
	repoZip       []byte
}

// gatherReportContents loads config/state, opens the repo, and collects
// everything Build needs to write out — without touching opts.OutDir. This
// keeps failures (in particular ErrRepoTooLarge) from CompressRepo or any
// other step from leaving partial output on disk.
func gatherReportContents(opts BuildOptions, version string) (*reportContents, error) {
	cfg, err := config.Load(opts.CfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, fmt.Errorf("opening repo: %w", err)
	}
	branch, _ := r.CurrentBranch()

	repoZip, err := CompressRepo(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, err
	}

	hosts, err := EnumerateHosts(r, branch)
	if err != nil {
		return nil, fmt.Errorf("enumerating hosts: %w", err)
	}
	hostsJSON, err := json.MarshalIndent(hosts, "", "  ")
	if err != nil {
		return nil, err
	}

	// The event log is already one JSON object per line on disk — embed it
	// verbatim as state_transitions.log rather than re-parsing and
	// re-marshaling it.
	eventLogBytes, err := os.ReadFile(eventlog.PathFor(opts.StatePath))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading event log: %w", err)
	}

	sum := summary{
		Time:        time.Now(),
		HDFVersion:  version,
		Trigger:     opts.Trigger,
		UserText:    opts.UserText,
		CrashDetail: opts.CrashDetail,
		Branch:      branch,
	}
	summaryJSON, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return nil, err
	}

	redactedCfg := *cfg
	redactedCfg.GitPushTarget = redactURL(cfg.GitPushTarget)
	var cfgBuf bytes.Buffer
	if err := toml.NewEncoder(&cfgBuf).Encode(redactedCfg); err != nil {
		return nil, fmt.Errorf("encoding redacted config: %w", err)
	}
	cfgBytes := cfgBuf.Bytes()
	stateBytes, err := os.ReadFile(opts.StatePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	return &reportContents{
		time:          sum.Time,
		summaryJSON:   summaryJSON,
		hostsJSON:     hostsJSON,
		eventLogBytes: eventLogBytes,
		cfgBytes:      cfgBytes,
		stateBytes:    stateBytes,
		repoZip:       repoZip,
	}, nil
}

// writeReportZip creates outPath and writes rc's contents into it as a zip
// archive. repo.zip is stored uncompressed since it's already
// deflate-compressed by CompressRepo.
func writeReportZip(outPath string, rc *reportContents) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	zw := zip.NewWriter(f)
	plainFiles := []struct {
		name string
		data []byte
	}{
		{"summary.json", rc.summaryJSON},
		{"hosts.json", rc.hostsJSON},
		{"state_transitions.log", rc.eventLogBytes},
		{configEntryName, rc.cfgBytes},
		{"state.toml", rc.stateBytes},
	}
	for _, file := range plainFiles {
		w, err := zw.Create(file.name)
		if err != nil {
			return err
		}
		if _, err := w.Write(file.data); err != nil {
			return err
		}
	}
	rw, err := zw.CreateHeader(&zip.FileHeader{Name: "repo.zip", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := rw.Write(rc.repoZip); err != nil {
		return err
	}
	return zw.Close()
}

// Build assembles a diagnostic report and writes it to a timestamped .zip in
// opts.OutDir, returning its path. The report contains: a summary of the
// trigger and any user-provided text, the rolling state-transition event
// log, every known host-* branch with its current SHA, config/state
// snapshots, and the backing dotfiles git repo (all branches + HEAD,
// compressed). Returns ErrRepoTooLarge — without writing anything — if the
// compressed repo exceeds MaxRepoZipBytes.
func Build(opts BuildOptions, version string) (string, error) {
	rc, err := gatherReportContents(opts, version)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}
	outPath := filepath.Join(opts.OutDir, fmt.Sprintf("hdf-report-%s.zip", rc.time.Format("20060102-150405")))
	if err := writeReportZip(outPath, rc); err != nil {
		return "", err
	}
	return outPath, nil
}

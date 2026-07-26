// report/report.go
package report

import (
	"archive/zip"
	"bytes"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hdf/config"
	"hdf/eventlog"
	"hdf/repo"
	"io"
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

// writeZipTo writes rc's contents into w as a zip archive. repo.zip is
// stored uncompressed since it's already deflate-compressed by
// CompressRepo. Factored out of writeReportZip so the archive-building
// logic is independently testable against any io.Writer, including one
// that injects a write failure.
func writeZipTo(w io.Writer, rc *reportContents) error {
	zw := zip.NewWriter(w)
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
		fw, err := zw.Create(file.name)
		if err != nil {
			return err
		}
		if _, err := fw.Write(file.data); err != nil {
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

// writeReportZip writes rc's contents to a uniquely-named temporary file in
// outPath's directory, then atomically renames it to outPath only once the
// archive has been fully and successfully written. This avoids leaving a
// partial or corrupt zip at the final path if a write fails partway
// through — the temp file is removed instead. The temp file's own
// uniqueness (via os.CreateTemp) also means two concurrent or same-second
// build attempts never clobber each other's in-progress output.
func writeReportZip(outPath string, rc *reportContents) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(outPath), ".hdf-report-*.zip.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if err = writeZipTo(tmp, rc); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, outPath); err != nil {
		return err
	}
	return nil
}

// randomHex returns a short random hex string for disambiguating filenames
// generated within the same second. Falls back to a nanosecond timestamp in
// the vanishingly unlikely event the system CSPRNG is unavailable, so
// Build never fails outright just because a filename couldn't be
// randomized.
func randomHex() string {
	b := make([]byte, 4)
	if _, err := crand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
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
	outPath := filepath.Join(opts.OutDir, fmt.Sprintf("hdf-report-%s-%s.zip", rc.time.Format("20060102-150405"), randomHex()))
	if err := writeReportZip(outPath, rc); err != nil {
		return "", err
	}
	return outPath, nil
}

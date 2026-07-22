// report/report_test.go
package report

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"hdf/config"
	"hdf/eventlog"
	"hdf/repo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testBranch is the branch name used by the fixture repos in this file's
// tests.
const testBranch = "main"

// setupReportFixture builds a minimal but real config/state/repo trio in a
// temp home dir and returns BuildOptions pointed at it.
func setupReportFixture(t *testing.T) BuildOptions {
	t.Helper()
	homeDir := t.TempDir()
	cfgPath := filepath.Join(homeDir, "config.toml")
	statePath := filepath.Join(homeDir, "state.toml")
	repoDir := filepath.Join(homeDir, "dotfiles")

	r, err := repo.Init(repoDir)
	if err != nil {
		t.Fatalf("repo.Init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("f.txt", "initial"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	cfg := &config.Config{LocalDotfilesDir: repoDir, Branch: testBranch}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("config.Save: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{LastCommit: "abc123"}); err != nil {
		t.Fatalf("config.SaveState: %v", err)
	}
	if err := eventlog.Append(eventlog.PathFor(statePath), "daemon_sync_success", ""); err != nil {
		t.Fatalf("eventlog.Append: %v", err)
	}

	return BuildOptions{
		CfgPath:   cfgPath,
		StatePath: statePath,
		Trigger:   TriggerManual,
		UserText:  "expected X, got Y",
		OutDir:    filepath.Join(homeDir, "out"),
	}
}

func TestBuild_CreatesZipWithExpectedEntries(t *testing.T) {
	opts := setupReportFixture(t)

	path, err := Build(opts, "1.2.3")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("zip.OpenReader(%s): %v", path, err)
	}
	defer func() { _ = zr.Close() }()

	names := map[string]*zip.File{}
	for _, f := range zr.File {
		names[f.Name] = f
	}
	for _, want := range []string{"summary.json", "hosts.json", "state_transitions.log", configEntryName, "state.toml", "repo.zip"} {
		if _, ok := names[want]; !ok {
			t.Errorf("zip missing entry %q; got entries %v", want, names)
		}
	}

	rc, err := names["summary.json"].Open()
	if err != nil {
		t.Fatalf("opening summary.json: %v", err)
	}
	defer func() { _ = rc.Close() }()
	var sum summary
	if err := json.NewDecoder(rc).Decode(&sum); err != nil {
		t.Fatalf("decoding summary.json: %v", err)
	}
	if sum.Trigger != TriggerManual || sum.UserText != "expected X, got Y" || sum.HDFVersion != "1.2.3" || sum.Branch != testBranch {
		t.Errorf("summary = %+v, unexpected fields", sum)
	}
}

func TestBuild_RepoTooLargeReturnsErrorAndWritesNothing(t *testing.T) {
	opts := setupReportFixture(t)
	bigDir := filepath.Join(filepath.Dir(opts.CfgPath), "dotfiles", ".git", "objects")
	if err := os.MkdirAll(bigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 5*1024*1024)
	if _, err := rand.Read(big); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bigDir, "big.pack"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Build(opts, "1.2.3")
	if !errors.Is(err, ErrRepoTooLarge) {
		t.Fatalf("Build err = %v, want ErrRepoTooLarge", err)
	}
	entries, err := os.ReadDir(opts.OutDir)
	if err == nil && len(entries) != 0 {
		t.Errorf("OutDir has %d entries, want 0 (nothing should be written on ErrRepoTooLarge)", len(entries))
	}
}

func TestBuild_RedactsConfigCredentials(t *testing.T) {
	opts := setupReportFixture(t)
	// Overwrite the fixture's config.toml with one that has credentials
	// in GitPushTarget.
	cfg := &config.Config{
		LocalDotfilesDir: filepath.Dir(opts.CfgPath) + "/dotfiles",
		Branch:           testBranch,
		GitPushTarget:    testCredentialedURL,
	}
	if err := config.Save(opts.CfgPath, cfg); err != nil {
		t.Fatalf("config.Save: %v", err)
	}

	path, err := Build(opts, "1.2.3")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("zip.OpenReader(%s): %v", path, err)
	}
	defer func() { _ = zr.Close() }()

	var cfgEntry *zip.File
	for _, f := range zr.File {
		if f.Name == configEntryName {
			cfgEntry = f
		}
	}
	if cfgEntry == nil {
		t.Fatal("zip missing config.toml entry")
	}
	rc, err := cfgEntry.Open()
	if err != nil {
		t.Fatalf("opening config.toml entry: %v", err)
	}
	defer func() { _ = rc.Close() }()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(rc); err != nil {
		t.Fatalf("reading config.toml entry: %v", err)
	}
	content := buf.String()
	if strings.Contains(content, "user:token") {
		t.Errorf("config.toml entry still contains credentials:\n%s", content)
	}
	if !strings.Contains(content, testRedactedURL) {
		t.Errorf("config.toml entry missing redacted URL:\n%s", content)
	}
}

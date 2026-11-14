//go:build e2e

package e2e

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRemoteIsBehind checks the "is behind" relation the branch-on-remote step
// asserts: the remote has the branch, and its commit is a strict ancestor of
// the local one — not missing, equal, ahead or diverged.
func TestRemoteIsBehind(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir,
			"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "base")
	base := git("rev-parse", "HEAD")
	git("commit", "-q", "--allow-empty", "-m", "local")
	local := git("rev-parse", "HEAD")
	git("checkout", "-q", "-b", "other", base)
	git("commit", "-q", "--allow-empty", "-m", "other")
	diverged := git("rev-parse", "HEAD")

	cases := []struct {
		name      string
		localSHA  string
		remoteSHA string // "" = the remote has no such branch
		wantOK    bool
	}{
		{"behind", local, base, true},
		{"missing", local, "", false},
		{"up to date", local, local, false},
		{"ahead", base, local, false},
		{"diverged", local, diverged, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := remoteIsBehind(dir, c.localSHA, c.remoteSHA)
			if c.wantOK && err != nil {
				t.Errorf("want behind, got %v", err)
			}
			if !c.wantOK && err == nil {
				t.Error("want an error, got none")
			}
		})
	}
}

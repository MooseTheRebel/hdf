// report/hosts.go
package report

import (
	"hdf/repo"
	"sort"
	"strings"
)

// hostBranchPrefix matches the "host-" naming convention branchName() in
// internal/cli/cli.go uses for each machine's git branch.
const hostBranchPrefix = "host-"

// HostInfo describes one known host-* branch and its current commit.
type HostInfo struct {
	Branch    string `json:"branch"`
	SHA       string `json:"sha"`
	IsCurrent bool   `json:"is_current"`
}

// EnumerateHosts returns one HostInfo per known host-* branch — local
// branches plus origin's remote-tracking branches, deduplicated (a local
// branch's SHA wins over its remote-tracking counterpart as the fresher
// value) — sorted by branch name. currentBranch marks which entry, if any,
// is this machine.
func EnumerateHosts(r *repo.Repo, currentBranch string) ([]HostInfo, error) {
	local, err := r.LocalBranches()
	if err != nil {
		return nil, err
	}
	remote, err := r.RemoteTrackingBranches("origin")
	if err != nil {
		return nil, err
	}

	shas := make(map[string]string)
	for _, b := range remote {
		if !strings.HasPrefix(b, hostBranchPrefix) {
			continue
		}
		if sha, err := r.RemoteBranchSHA("origin", b); err == nil {
			shas[b] = sha
		}
	}
	for _, b := range local {
		if !strings.HasPrefix(b, hostBranchPrefix) {
			continue
		}
		if sha, err := r.BranchSHA(b); err == nil {
			shas[b] = sha
		}
	}

	names := make([]string, 0, len(shas))
	for b := range shas {
		names = append(names, b)
	}
	sort.Strings(names)

	hosts := make([]HostInfo, 0, len(names))
	for _, b := range names {
		hosts = append(hosts, HostInfo{Branch: b, SHA: shas[b], IsCurrent: b == currentBranch})
	}
	return hosts, nil
}

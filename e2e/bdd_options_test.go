//go:build e2e

package e2e

import (
	"path/filepath"
	"testing"
)

func TestBDDOptions(t *testing.T) {
	defaultReport := filepath.Join("..", "build", "coverage", "state-graph.md")
	cases := []struct {
		name       string
		env        map[string]string
		wantTags   string
		wantSubset bool
		wantReport string // "" = don't write a report
	}{
		{"full run", nil, defaultTags, false, defaultReport},
		{"empty tags is a full run", map[string]string{"HDF_BDD_TAGS": ""}, defaultTags, false, defaultReport},
		// A subset keeps the default exclusions it doesn't mention itself...
		{"excluding wip still excludes bug", map[string]string{"HDF_BDD_TAGS": "~@wip"}, "~@wip && ~@bug", true, ""},
		{"another tag keeps both exclusions", map[string]string{"HDF_BDD_TAGS": "@known-gap"}, "@known-gap && ~@wip && ~@bug", true, ""},
		// ...so naming one is how to run it.
		{"asking for bug runs bug", map[string]string{"HDF_BDD_TAGS": "@bug"}, "@bug && ~@wip", true, ""},
		{"bug in an or-group", map[string]string{"HDF_BDD_TAGS": "@bug,@wip"}, "@bug,@wip", true, ""},
		// A subset only writes a report where it's told to, so it never
		// overwrites the full run's.
		{"subset with explicit report", map[string]string{"HDF_BDD_TAGS": "@bug", "HDF_BDD_REPORT": "x.md"}, "@bug && ~@wip", true, "x.md"},
		{"full run with explicit report", map[string]string{"HDF_BDD_REPORT": "x.md"}, defaultTags, false, "x.md"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := c.env[k]; return v, ok }
			tags, subset, report := bddOptions(lookup)
			if tags != c.wantTags || subset != c.wantSubset || report != c.wantReport {
				t.Errorf("got (%q, %v, %q), want (%q, %v, %q)", tags, subset, report, c.wantTags, c.wantSubset, c.wantReport)
			}
		})
	}
}

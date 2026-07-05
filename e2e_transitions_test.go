//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type Transition struct {
	Name string
	From FileState
	Via  string // human-readable description of the command/action
	To   FileState
}

// validTransitions is the contract table for the state machine.
// Each entry with a matching case in TestTransitionContractCoverage is exercised.
//
// Observer transitions (marked with "foreign-" prefix) cannot be exercised in
// TestTransitionContractCoverage — no local command produces them. They are
// exercised by TestExternalTransitions in e2e_promote_guards_test.go.
//
// Known gap: RegistryOnly has no outgoing transitions here. It is reachable in
// deriveFileState (registry entry exists but both branches have no content) but
// no hdf command currently produces it in a normal workflow. If an unenroll/remove
// command is added, it will need entries here.
var validTransitions = []Transition{
	{"enroll", Untracked, "changes-push", Enrolled},
	{"promote", Enrolled, "hdf promote", Synced},
	{"pull-accept", Promoted, "changes-pull → accept", Synced},
	{"pull-skip", Promoted, "changes-pull → skip", Promoted},
	{"re-enroll", Synced, "edit + changes-push", Diverged},
	{"re-promote", Diverged, "hdf promote", Synced},
	{"pull-accept-diverged", Diverged, "changes-pull → accept", Synced},
	// Observer transitions — no local command; exercised by TestExternalTransitions.
	{"foreign-promote-same-file", Untracked, "another machine promotes", Promoted},
	{"foreign-update-diverges-synced", Synced, "another machine re-promotes", Diverged},
}

// TestTransitionContractCoverage re-runs each transition in isolation and
// asserts the expected To state, verifying the state machine contract.
func TestTransitionContractCoverage(t *testing.T) {
	for _, tr := range validTransitions {
		tr := tr
		if strings.HasPrefix(tr.Name, "foreign-") {
			continue // observer transitions; exercised by TestExternalTransitions
		}
		t.Run(tr.Name, func(t *testing.T) {
			t.Parallel()
			nodes, _ := setupCluster(t, 2)
			nodeA := nodes[0]
			dotfile := filepath.Join(nodeA.home, ".bashrc")

			switch tr.Name {
			case "enroll":
				if err := os.WriteFile(dotfile, []byte("x\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				assertFileState(t, nodeA, "~/.bashrc", Untracked)
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("changes-push: %s", stderr)
				}
				assertFileState(t, nodeA, "~/.bashrc", Enrolled)

			case "promote":
				if err := os.WriteFile(dotfile, []byte("x\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("changes-push: %s", stderr)
				}
				assertFileState(t, nodeA, "~/.bashrc", Enrolled)
				hdfPromote(t, nodeA)
				assertFileState(t, nodeA, "~/.bashrc", Synced)

			case "pull-accept":
				nodeB := nodes[1]
				if err := os.WriteFile(dotfile, []byte("x\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("A changes-push: %s", stderr)
				}
				hdfPromote(t, nodeA)
				// Fetch so B sees the promoted content before asserting From state.
				runHDFNode(t, nodeB, "n\n", "changes-pull") //nolint:errcheck
				assertFileState(t, nodeB, "~/.bashrc", Promoted)
				if _, stderr, code := runHDFNode(t, nodeB, "y\n", "changes-pull"); code != 0 {
					t.Fatalf("B changes-pull accept: %s", stderr)
				}
				assertFileState(t, nodeB, "~/.bashrc", Synced)

			case "pull-skip":
				nodeB := nodes[1]
				if err := os.WriteFile(dotfile, []byte("x\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("A changes-push: %s", stderr)
				}
				hdfPromote(t, nodeA)
				// Fetch so B sees the promoted content before asserting From state.
				runHDFNode(t, nodeB, "n\n", "changes-pull") //nolint:errcheck
				assertFileState(t, nodeB, "~/.bashrc", Promoted)
				// Skip (decline to accept main's content); state stays Promoted.
				if _, _, code := runHDFNode(t, nodeB, "n\n", "changes-pull"); code != 0 {
					t.Fatalf("B changes-pull skip failed")
				}
				assertFileState(t, nodeB, "~/.bashrc", Promoted)

			case "re-enroll":
				if err := os.WriteFile(dotfile, []byte("v1\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("changes-push v1: %s", stderr)
				}
				hdfPromote(t, nodeA)
				assertFileState(t, nodeA, "~/.bashrc", Synced)
				if err := os.WriteFile(dotfile, []byte("v2\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("changes-push v2: %s", stderr)
				}
				assertFileState(t, nodeA, "~/.bashrc", Diverged)

			case "re-promote":
				if err := os.WriteFile(dotfile, []byte("v1\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("changes-push v1: %s", stderr)
				}
				hdfPromote(t, nodeA)
				if err := os.WriteFile(dotfile, []byte("v2\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("changes-push v2: %s", stderr)
				}
				assertFileState(t, nodeA, "~/.bashrc", Diverged)
				hdfPromote(t, nodeA)
				assertFileState(t, nodeA, "~/.bashrc", Synced)

			case "pull-accept-diverged":
				nodeB := nodes[1]

				// B enrolls .bashrc with its own content → Enrolled on B's branch.
				dotfileB := filepath.Join(nodeB.home, ".bashrc")
				if err := os.WriteFile(dotfileB, []byte("B-content\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeB, "", "changes-push", "--yes", dotfileB); code != 0 {
					t.Fatalf("B changes-push: %s", stderr)
				}

				// A enrolls and promotes .bashrc (different content).
				if err := os.WriteFile(dotfile, []byte("A-content\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
					t.Fatalf("A changes-push: %s", stderr)
				}
				hdfPromote(t, nodeA)

				// B fetches to see A's promotion; B has B-content, main has A-content → Diverged.
				runHDFNode(t, nodeB, "n\n", "changes-pull") //nolint:errcheck
				assertFileState(t, nodeB, "~/.bashrc", Diverged)

				// B accepts A's content → Synced.
				if _, stderr, code := runHDFNode(t, nodeB, "y\n", "changes-pull"); code != 0 {
					t.Fatalf("B changes-pull accept from Diverged: %s", stderr)
				}
				assertFileState(t, nodeB, "~/.bashrc", Synced)

			default:
				t.Fatalf("unhandled transition %q — add a case", tr.Name)
			}
		})
	}
}

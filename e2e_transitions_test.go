//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"testing"
)

type Transition struct {
	Name string
	From FileState
	Via  string // human-readable description of the command/action
	To   FileState
}

// validTransitions is the contract table for the state machine.
// Each entry must have a corresponding exercising scenario in TestTransitionContractCoverage.
var validTransitions = []Transition{
	{"enroll", Untracked, "changes-push", Enrolled},
	{"promote", Enrolled, "hdf promote", Synced},
	{"pull-accept", Promoted, "changes-pull → accept", Synced},
	{"pull-skip", Promoted, "changes-pull → skip", Promoted},
	{"re-enroll", Synced, "edit + changes-push", Diverged},
	{"re-promote", Diverged, "hdf promote", Synced},
}

// TestTransitionContractCoverage re-runs each transition in isolation and
// asserts the expected To state, verifying the state machine contract.
func TestTransitionContractCoverage(t *testing.T) {
	for _, tr := range validTransitions {
		tr := tr
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
				// Fetch, then assert From state, then skip.
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

			default:
				t.Fatalf("unhandled transition %q — add a case", tr.Name)
			}
		})
	}
}

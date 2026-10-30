//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"hdf/config"
	"hdf/repo"
)

const q = `"([^"]*)"` // a quoted step argument

// how is the optional tail on actions: "using the GUI" (desktops only) or
// "without a terminal" (nobody there to answer, e.g. a cron job).
const how = `( using the GUI)?( without a terminal)?`

var states = map[string]FileState{}

func init() {
	for s := Untracked; s <= RegistryOnly; s++ {
		states[s.String()] = s
	}
}

func registerSteps(sc *godog.ScenarioContext) {
	// Setting the scene.
	sc.Given(`^a remote dotfiles repository$`, aRemote)
	sc.Given(`^a (desktop machine|headless server) `+q+`$`, aMachine)
	sc.Given(`^`+q+` has a file `+q+` containing `+q+`$`, hasFile)
	sc.Given(`^`+q+` is offline$`, goesOffline)

	// Things I do.
	sc.When(`^I edit `+q+` on `+q+` to contain `+q+`$`, iEdit)
	sc.When(`^I (try to )?push `+q+` from `+q+how+`$`, iPush)
	sc.When(`^I (try to )?promote from `+q+how+`$`, iPromote)
	sc.When(`^I (try to )?promote from `+q+` and (keep|overwrite) main's newer `+q+`$`, iPromoteDeciding)
	sc.When(`^I (try to )?promote from `+q+` and continue past files I haven't pulled$`, iPromotePastUnpulled)
	sc.When(`^I (try to )?review incoming changes on `+q+how+` and accept everything$`, iPullAll)
	sc.When(`^I review incoming changes on `+q+how+` and (accept|skip) `+q+`(?: (?:and|but) (accept|skip) `+q+`)?$`, iPullChoosing)
	sc.When(`^I merge `+q+`'s branch on GitHub$`, iMergeOnGitHub)
	sc.When(`^`+q+` goes offline$`, goesOffline)
	sc.When(`^`+q+` comes back online$`, comesOnline)
	sc.When(`^the daemon on `+q+` runs one sync cycle$`, daemonCycle)
	sc.When(`^I reinstall `+q+` and (reuse|rename) its existing branch$`, iReinstall)
	sc.When(`^I retire `+q+`$`, iRetire)
	sc.When(`^I acknowledge the daemon's warnings on `+q+`$`, iAcknowledge)

	// What I expect to see.
	sc.Then(`^`+q+` is (Untracked|Enrolled|Promoted|Synced|Diverged|RegistryOnly) on `+q+`$`, fileIs)
	sc.Then(`^`+q+` on `+q+` contains `+q+`$`, fileContains)
	sc.Then(`^the remote main has `+q+` containing `+q+`$`, remoteHas)
	sc.Then(`^the remote main does not have `+q+`$`, remoteLacks)
	sc.Then(`^the last command on `+q+` (failed|succeeded)(?: mentioning `+q+`)?$`, lastCommand)
	sc.Then(`^`+q+`'s branch on the remote (is behind|matches|is missing) (?:its|the) local branch$`, branchOnRemote)
	sc.Then(`^the daemon on `+q+` (warned|did not warn) me(?: about `+q+`)?$`, daemonWarned)
}

// ---- Given ----

func aRemote(ctx context.Context) error {
	w := worldFrom(ctx)
	w.bareDir = filepath.Join(w.root, "remote.git")
	return os.MkdirAll(w.bareDir, 0o755)
}

func aMachine(ctx context.Context, kind, name string) error {
	p := desktop
	if kind == "headless server" {
		p = server
	}
	return worldFrom(ctx).addMachine(name, p)
}

func hasFile(ctx context.Context, name, file, content string) error {
	w := worldFrom(ctx)
	m, err := w.machine(name)
	if err != nil {
		return err
	}
	w.track(file)
	return writeDotfile(m, file, content)
}

func writeDotfile(m *machine, file, content string) error {
	p := config.ExpandPathIn(file, m.node.home)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// Writes through hdf's symlink, as an editor would.
	return os.WriteFile(p, []byte(content+"\n"), 0o644)
}

// ---- When ----

// interactive returns respond, or nil when there's no terminal; it refuses
// the GUI on servers, and on desktops until the GUI driver exists.
func interactive(w *world, m *machine, gui, noTTY string, respond responder) (responder, error) {
	if gui != "" {
		if m.profile == server {
			return nil, fmt.Errorf("%s is a headless server: there's no GUI there", m.name)
		}
		if !w.tags["@gui-pending"] {
			return nil, fmt.Errorf("GUI steps aren't driven through the GUI yet; tag the scenario @gui-pending to run them over the CLI (tags: %v)", w.sortedTags())
		}
	}
	if noTTY != "" {
		return nil, nil
	}
	return respond, nil
}

// expect fails the step when the command couldn't be driven, or failed when
// I didn't say I was only trying.
func expect(m *machine, trying string) error {
	if m.lastErr != nil {
		return m.lastErr
	}
	if trying == "" && m.last.code != 0 {
		return fmt.Errorf("the command on %s failed (exit %d); say \"I try to ...\" if that's expected:\n%s", m.name, m.last.code, m.last.output())
	}
	return nil
}

func nothing(string) (string, bool) { return "", false }

func iEdit(ctx context.Context, file, name, content string) error {
	w := worldFrom(ctx)
	w.track(file)
	return w.act(name, "edit", nil, func(m *machine) error { return writeDotfile(m, file, content) })
}

func iPush(ctx context.Context, trying, file, name, gui, noTTY string) error {
	w := worldFrom(ctx)
	w.track(file)
	return w.act(name, "push", nil, func(m *machine) error {
		respond, err := interactive(w, m, gui, noTTY, nothing)
		if err != nil {
			return err
		}
		w.hdf(m, respond, "changes-push", "--yes", config.ExpandPathIn(file, m.node.home))
		return expect(m, trying)
	})
}

func iPromote(ctx context.Context, trying, name, gui, noTTY string) error {
	return promoteWith(ctx, trying, name, gui, noTTY, nothing)
}

func iPromoteDeciding(ctx context.Context, trying, name, decision, file string) error {
	answer := "n" // keep main's newer version
	if decision == "overwrite" {
		answer = "y"
	}
	return promoteWith(ctx, trying, name, "", "", func(prompt string) (string, bool) {
		if strings.HasPrefix(prompt, "Overwrite main's newer version of "+file) {
			return answer, true
		}
		return "", false
	})
}

func iPromotePastUnpulled(ctx context.Context, trying, name string) error {
	return promoteWith(ctx, trying, name, "", "", answerContaining("Continue promoting?", "y"))
}

func promoteWith(ctx context.Context, trying, name, gui, noTTY string, respond responder) error {
	w := worldFrom(ctx)
	return w.act(name, "promote", nil, func(m *machine) error {
		respond, err := interactive(w, m, gui, noTTY, respond)
		if err != nil {
			return err
		}
		w.hdf(m, respond, "promote")
		return expect(m, trying)
	})
}

func iPullAll(ctx context.Context, trying, name, gui, noTTY string) error {
	return pull(ctx, trying, name, gui, noTTY, nil, nil, true)
}

func iPullChoosing(ctx context.Context, name, gui, noTTY, verb1, file1, verb2, file2 string) error {
	var accept, skip []string
	for _, c := range [][2]string{{verb1, file1}, {verb2, file2}} {
		switch c[0] {
		case "accept":
			accept = append(accept, c[1])
		case "skip":
			skip = append(skip, c[1])
		}
	}
	return pull(ctx, "", name, gui, noTTY, accept, skip, false)
}

func pull(ctx context.Context, trying, name, gui, noTTY string, accept, skip []string, acceptRest bool) error {
	w := worldFrom(ctx)
	actionFor := func(file string) string {
		for _, f := range accept {
			if f == file {
				return "pull-accept"
			}
		}
		for _, f := range skip {
			if f == file {
				return "pull-skip"
			}
		}
		if acceptRest {
			return "pull-accept"
		}
		return "pull"
	}
	return w.act(name, "pull", actionFor, func(m *machine) error {
		respond, err := interactive(w, m, gui, noTTY, fileAnswers(accept, skip, acceptRest))
		if err != nil {
			return err
		}
		w.hdf(m, respond, "changes-pull")
		return expect(m, trying)
	})
}

func iMergeOnGitHub(ctx context.Context, name string) error {
	w := worldFrom(ctx)
	return w.act(name, "pr-merge", nil, func(m *machine) error {
		dir, err := os.MkdirTemp(w.root, "pr-")
		if err != nil {
			return err
		}
		return prMergeIn(dir, m.node, "file://"+w.bareDir)
	})
}

func goesOffline(ctx context.Context, name string) error {
	w := worldFrom(ctx)
	m, err := w.machine(name)
	if err != nil {
		return err
	}
	return w.setOnline(m, false)
}

func comesOnline(ctx context.Context, name string) error {
	w := worldFrom(ctx)
	m, err := w.machine(name)
	if err != nil {
		return err
	}
	return w.setOnline(m, true)
}

func daemonCycle(ctx context.Context, name string) error {
	w := worldFrom(ctx)
	return w.act(name, "daemon-sync", nil, w.runDaemonCycle)
}

func iReinstall(ctx context.Context, name, choice string) error {
	w := worldFrom(ctx)
	answer := "1"
	if choice == "rename" {
		answer = "2"
	}
	return w.act(name, "reinstall-"+choice, nil, func(m *machine) error {
		// A fresh OS: nothing of the old home survives.
		if err := os.RemoveAll(m.node.home); err != nil {
			return err
		}
		if err := os.MkdirAll(m.node.home, 0o755); err != nil {
			return err
		}
		return w.initMachine(m, false, answer)
	})
}

func iRetire(ctx context.Context, name string) error {
	w := worldFrom(ctx)
	m, err := w.machine(name)
	if err != nil {
		return err
	}
	m.retired = true
	w.steps = append(w.steps, journeyStep{name, "retire"})
	return nil
}

func iAcknowledge(ctx context.Context, name string) error {
	m, err := worldFrom(ctx).machine(name)
	if err != nil {
		return err
	}
	m.ackWarn = true
	return nil
}

// ---- Then ----

func fileIs(ctx context.Context, file, state, name string) error {
	w := worldFrom(ctx)
	m, err := w.machine(name)
	if err != nil {
		return err
	}
	w.track(file)
	got, err := w.stateOf(m, file)
	if err != nil {
		return err
	}
	if got != states[state] {
		return fmt.Errorf("%s is %s on %s, not %s", file, got, name, state)
	}
	return nil
}

func fileContains(ctx context.Context, file, name, content string) error {
	m, err := worldFrom(ctx).machine(name)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(config.ExpandPathIn(file, m.node.home))
	if err != nil {
		return err
	}
	if strings.TrimSuffix(string(b), "\n") != content {
		return fmt.Errorf("%s on %s contains %q, not %q", file, name, b, content)
	}
	return nil
}

func remoteFile(w *world, file string) ([]byte, error) {
	if len(w.order) == 0 {
		return nil, errors.New("no machines")
	}
	rel, err := repoRelPath(w.machines[w.order[0]], file)
	if err != nil {
		return nil, err
	}
	r, err := w.remote()
	if err != nil {
		return nil, err
	}
	return r.ReadFileFromBranch("main", rel)
}

func remoteHas(ctx context.Context, file, content string) error {
	b, err := remoteFile(worldFrom(ctx), file)
	if err != nil {
		return err
	}
	if b == nil {
		return fmt.Errorf("the remote main has no %s", file)
	}
	if strings.TrimSuffix(string(b), "\n") != content {
		return fmt.Errorf("the remote main's %s contains %q, not %q", file, b, content)
	}
	return nil
}

func remoteLacks(ctx context.Context, file string) error {
	b, err := remoteFile(worldFrom(ctx), file)
	if err != nil {
		return err
	}
	if b != nil {
		return fmt.Errorf("the remote main has %s (%q)", file, b)
	}
	return nil
}

func lastCommand(ctx context.Context, name, outcome, mention string) error {
	m, err := worldFrom(ctx).machine(name)
	if err != nil {
		return err
	}
	if m.lastErr != nil {
		return m.lastErr
	}
	if failed := m.last.code != 0; failed != (outcome == "failed") {
		return fmt.Errorf("the last command on %s exited %d, but I expected it to have %s:\n%s", name, m.last.code, outcome, m.last.output())
	}
	if mention != "" && !strings.Contains(m.last.output(), mention) {
		return fmt.Errorf("the last command on %s didn't mention %q:\n%s", name, mention, m.last.output())
	}
	return nil
}

func branchOnRemote(ctx context.Context, name, relation string) error {
	w := worldFrom(ctx)
	m, err := w.machine(name)
	if err != nil {
		return err
	}
	cfg, err := loadNodeConfig(m.node)
	if err != nil {
		return err
	}
	local, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return err
	}
	localSHA, err := local.BranchSHA(cfg.Branch)
	if err != nil {
		return err
	}
	remote, err := w.remote()
	if err != nil {
		return err
	}
	// A branch the remote doesn't have is an answer here, not an error.
	remoteSHA, missing := "", true
	if sha, lookupErr := remote.BranchSHA(cfg.Branch); lookupErr == nil {
		remoteSHA, missing = sha, false
	}
	switch relation {
	case "is missing":
		if !missing {
			return fmt.Errorf("the remote has %s's branch %s", name, cfg.Branch)
		}
	case "matches":
		if missing || remoteSHA != localSHA {
			return fmt.Errorf("the remote's %s is %q, local is %q", cfg.Branch, remoteSHA, localSHA)
		}
	case "is behind":
		if !missing && remoteSHA == localSHA {
			return fmt.Errorf("the remote's %s is up to date with %s", cfg.Branch, name)
		}
	}
	return nil
}

func daemonWarned(ctx context.Context, name, warned, about string) error {
	m, err := worldFrom(ctx).machine(name)
	if err != nil {
		return err
	}
	var notes []string
	for _, line := range strings.Split(m.lastDaemon, "\n") {
		if strings.HasPrefix(line, "hdf notification:") || strings.HasPrefix(line, "hdf alert:") {
			if about == "" || strings.Contains(line, about) {
				notes = append(notes, line)
			}
		}
	}
	if (warned == "warned") != (len(notes) > 0) {
		return fmt.Errorf("the daemon on %s %s me about %q; its output:\n%s", name, map[bool]string{true: "warned", false: "did not warn"}[len(notes) > 0], about, m.lastDaemon)
	}
	return nil
}

//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hdf/repo"
)

// profile is the kind of machine: it decides which interface I use there,
// not whether I can answer prompts — I'm at a terminal on both.
type profile int

const (
	desktop profile = iota // CLI or GUI
	server                 // headless: CLI only
)

func (p profile) String() string { return [...]string{"desktop", "server"}[p] }

// machine is one of my computers in a scenario.
type machine struct {
	name    string
	profile profile
	node    Node
	link    string // this machine's route to the remote; removed while offline
	online  bool
	retired bool
	ackWarn bool // answer "continue anyway" to the daemon's warnings next time

	last       actorRun // the last hdf command I ran here
	lastErr    error    // set when that command could not be driven at all
	lastDaemon string   // stderr from the last daemon cycle
}

// world is the state of one scenario: my machines, the shared remote, and the
// transitions the scenario walked.
type world struct {
	scenario string
	tags     map[string]bool
	root     string // temp dir holding everything; removed after the scenario
	bareDir  string // the remote ("GitHub")

	machines map[string]*machine
	order    []string // machines in the order they were added
	files    []string // tracked dotfiles ("~/.bashrc"), in first-mention order

	steps []journeyStep
	rec   *recorder
}

// journeyStep is one action, in order, for the cluster-level journey report.
type journeyStep struct{ machine, action string }

type worldKey struct{}

func worldFrom(ctx context.Context) *world { return ctx.Value(worldKey{}).(*world) }

func newWorld(scenario string, tags []string, rec *recorder) (*world, error) {
	// Resolve symlinks (macOS /var → /private/var) so paths hdf prints and
	// paths we compare against agree. Resolving the parent before creating
	// root means nothing can fail once root exists, so it can't be left behind.
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(tmp, "hdf-bdd-")
	if err != nil {
		return nil, err
	}
	w := &world{
		scenario: scenario,
		tags:     map[string]bool{},
		root:     root,
		machines: map[string]*machine{},
		rec:      rec,
	}
	for _, t := range tags {
		w.tags[t] = true
	}
	return w, nil
}

// cleanup removes the scenario's files. Pushes over file:// start git's
// background maintenance in the bare repo, so retry briefly (see gitTempDir).
func (w *world) cleanup() error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := os.RemoveAll(w.root)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (w *world) machine(name string) (*machine, error) {
	m, ok := w.machines[name]
	if !ok {
		return nil, fmt.Errorf("there is no machine called %q in this scenario", name)
	}
	return m, nil
}

func (w *world) track(file string) {
	for _, f := range w.files {
		if f == file {
			return
		}
	}
	w.files = append(w.files, file)
}

func (w *world) remoteURL(m *machine) string { return "file://" + m.link }

// addMachine sets up a new machine: the first one creates the remote, later
// ones clone it — the same as I'd run `hdf init` on each.
func (w *world) addMachine(name string, p profile) error {
	if w.bareDir == "" {
		return errors.New(`add "a remote dotfiles repository" before any machine`)
	}
	if _, ok := w.machines[name]; ok {
		return fmt.Errorf("machine %q already exists", name)
	}
	m := &machine{
		name:    name,
		profile: p,
		node:    Node{home: filepath.Join(w.root, "machines", name, "home"), branch: name},
		link:    filepath.Join(w.root, "net", name),
		online:  true,
	}
	if err := os.MkdirAll(m.node.home, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.link), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(w.bareDir, m.link); err != nil {
		return err
	}
	first := len(w.machines) == 0
	w.machines[name] = m
	w.order = append(w.order, name)

	if first {
		if err := w.initMachine(m, true, ""); err != nil {
			return err
		}
		// Local init doesn't push; seed the remote with main so others can clone.
		cfg, err := loadNodeConfig(m.node)
		if err != nil {
			return err
		}
		r, err := repo.Open(cfg.LocalDotfilesDir)
		if err != nil {
			return err
		}
		return r.Push("main")
	}
	return w.initMachine(m, false, "")
}

// initMachine runs `hdf init` on m. local creates a working copy whose push
// target is the remote; otherwise it clones. collision is my answer if hdf
// says the machine's branch already exists ("1" reuse, "2" unique); empty
// means I don't expect that question.
func (w *world) initMachine(m *machine, local bool, collision string) error {
	choices := 0
	respond := func(prompt string) (string, bool) {
		switch {
		case strings.HasPrefix(prompt, "Choice"):
			choices++
			if choices == 1 {
				if local {
					return "1", true
				}
				return "2", true
			}
			return collision, collision != ""
		case strings.HasPrefix(prompt, "Local repo path"):
			return filepath.Join(m.node.home, ".local", "share", "hdf", "repo"), true
		case strings.HasPrefix(prompt, "Push target path"):
			return m.link, true
		case strings.HasPrefix(prompt, "Remote repository URL"):
			return w.remoteURL(m), true
		}
		return "", false
	}
	run, err := runActor(m.node, nil, respond, "init")
	m.last, m.lastErr = run, err
	if err != nil {
		return err
	}
	if run.code != 0 {
		return fmt.Errorf("hdf init on %s failed (exit %d):\n%s", m.name, run.code, run.output())
	}
	return nil
}

// setOnline cuts or restores m's route to the remote.
func (w *world) setOnline(m *machine, online bool) error {
	if m.online == online {
		return nil
	}
	m.online = online
	if online {
		return os.Symlink(w.bareDir, m.link)
	}
	return os.Remove(m.link)
}

// hdf runs an hdf command on m as I would at its terminal. respond nil means
// there's no terminal (nobody to answer).
func (w *world) hdf(m *machine, respond responder, args ...string) {
	if respond != nil && m.ackWarn {
		respond = answerContaining("Continue anyway?", "y").orElse(respond)
		m.ackWarn = false
	}
	m.last, m.lastErr = runActor(m.node, nil, respond, args...)
}

// runDaemonCycle starts `hdf daemon run` on m, lets it finish one sync cycle,
// and stops it.
func (w *world) runDaemonCycle(m *machine) error {
	ctx, cancel := context.WithTimeout(context.Background(), actorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, hdfBin, "daemon", "run")
	cmd.Env = nodeEnv(m.node)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var out strings.Builder
	sc := bufio.NewScanner(stderr)
	done := false
	for sc.Scan() {
		out.WriteString(sc.Text() + "\n")
		if strings.HasPrefix(sc.Text(), "next sync in") {
			done = true
			break
		}
	}
	// Interrupt rather than kill: the daemon shuts down gracefully on SIGINT,
	// which also lets a coverage build write its data.
	_ = cmd.Process.Signal(os.Interrupt)
	stopped := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stderr)
		_ = cmd.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-stopped
	}
	m.lastDaemon = out.String()
	if !done {
		return fmt.Errorf("the daemon on %s stopped before finishing a sync cycle:\n%s", m.name, out.String())
	}
	return nil
}

// remote opens the shared bare repo — what's actually on "GitHub".
func (w *world) remote() (*repo.Repo, error) { return repo.Open(w.bareDir) }

// stateOf is file's state on m, judged against the remote's real main rather
// than what m last fetched — so checking never fetches on m's behalf.
func (w *world) stateOf(m *machine, file string) (FileState, error) {
	remote, err := w.remote()
	if err != nil {
		return 0, err
	}
	return fileStateAgainst(m.node, file, remote)
}

// snapshot is every tracked file's state on every machine still in use.
type snapshot map[string]map[string]FileState // machine → file → state

// It reads the remote's main and each machine's config and repo once, not
// once per file.
func (w *world) snapshot() (snapshot, error) {
	remote, err := w.remote()
	if err != nil {
		return nil, err
	}
	main := newMainView(func(rel string) ([]byte, error) { return remote.ReadFileFromBranch("main", rel) })
	s := snapshot{}
	for _, name := range w.order {
		m := w.machines[name]
		if m.retired {
			continue
		}
		s[name] = map[string]FileState{}
		if len(w.files) == 0 {
			continue
		}
		mv, err := loadMachineView(m.node)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		for _, f := range w.files {
			st, err := mv.stateOf(main, f)
			if err != nil {
				return nil, fmt.Errorf("state of %s on %s: %w", f, name, err)
			}
			s[name][f] = st
		}
	}
	return s, nil
}

// act runs one of my actions on machine name and records, for every tracked
// file on every machine, the transition it caused: role "me" on the machine I
// acted on, "elsewhere" on the others. actionFor names the action as it
// applies to one file on the acting machine (e.g. pull-accept vs pull-skip);
// nil uses action for every file.
func (w *world) act(name, action string, actionFor func(file string) string, fn func(m *machine) error) error {
	m, err := w.machine(name)
	if err != nil {
		return err
	}
	if !m.online {
		action += " (offline)"
	}
	before, err := w.snapshot()
	if err != nil {
		return err
	}
	if err := fn(m); err != nil {
		return err
	}
	after, err := w.snapshot()
	if err != nil {
		return err
	}
	w.steps = append(w.steps, journeyStep{name, action})
	for mach, files := range after {
		for f, to := range files {
			from, ok := before[mach][f]
			if !ok {
				continue // machine added by this step
			}
			e := edge{From: from, Action: action, Role: elsewhere, To: to}
			if mach == name {
				e.Role = me
				if actionFor != nil {
					e.Action = actionFor(f)
					if !m.online {
						e.Action += " (offline)"
					}
				}
			}
			w.rec.observe(e, w.scenario)
		}
	}
	return nil
}

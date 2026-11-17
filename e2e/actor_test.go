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
	"strings"
	"sync"
	"time"
)

// actorTimeout bounds one hdf invocation driven by an actor, so a prompt the
// actor never saw (and so never answered) fails the step instead of hanging.
const actorTimeout = 2 * time.Minute

// promptMarker is what hdf writes after each prompt when it's set as
// HDF_PROMPT_MARKER: it ends the prompt's line, so a line ending in it is
// exactly a prompt hdf is waiting on.
const promptMarker = "\x1ehdf-prompt\x1e\n"

// responder decides the reply to one prompt (the text of its line). ok=false
// means the actor did not expect this prompt; the run then fails.
type responder func(prompt string) (answer string, ok bool)

// actorRun is the outcome of one hdf invocation.
type actorRun struct {
	stdout, stderr string
	code           int
	prompts        []string // every prompt seen, in order
	unexpected     []string // prompts the responder refused
}

// output is stdout and stderr together, for "mentioning ..." checks.
func (r actorRun) output() string { return r.stdout + r.stderr }

// runActor runs hdf on node as a person at a terminal would: it watches stdout
// and stderr and answers each prompt as respond decides, so answers follow the
// prompt's content (e.g. the file it names) rather than a fixed order. A nil
// respond runs hdf with no input at all — like a cron job or a service.
func runActor(node Node, extraEnv []string, respond responder, args ...string) (actorRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), actorTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, hdfBin, args...)
	cmd.Env = append(append(nodeEnv(node), "HDF_PROMPT_MARKER="+promptMarker), extraEnv...)

	var stdin io.WriteCloser
	if respond != nil {
		var err error
		if stdin, err = cmd.StdinPipe(); err != nil {
			return actorRun{}, err
		}
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return actorRun{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return actorRun{}, err
	}
	if err := cmd.Start(); err != nil {
		return actorRun{}, err
	}

	var (
		mu  sync.Mutex
		run actorRun
		wg  sync.WaitGroup
	)
	// answer is called with each prompt watchPrompts finds.
	answer := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		prompt := strings.TrimSpace(line)
		run.prompts = append(run.prompts, prompt)
		if respond == nil {
			return
		}
		reply, ok := respond(prompt)
		if !ok {
			run.unexpected = append(run.unexpected, prompt)
			_ = stdin.Close() // EOF: hdf aborts instead of waiting forever
			return
		}
		_, _ = io.WriteString(stdin, reply+"\n")
	}
	watch := func(r io.Reader, into *strings.Builder) {
		defer wg.Done()
		watchPrompts(r, func(chunk string) {
			mu.Lock()
			into.WriteString(chunk)
			mu.Unlock()
		}, answer)
	}
	var stdoutBuf, stderrBuf strings.Builder
	wg.Add(2)
	go watch(stdoutPipe, &stdoutBuf)
	go watch(stderrPipe, &stderrBuf)
	wg.Wait()

	waitErr := cmd.Wait()
	if stdin != nil {
		_ = stdin.Close()
	}
	run.stdout, run.stderr = stdoutBuf.String(), stderrBuf.String()
	if ctx.Err() != nil {
		return run, fmt.Errorf("hdf %s timed out after %s; last prompts %q\n%s", strings.Join(args, " "), actorTimeout, run.prompts, run.output())
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(waitErr, &exitErr):
		run.code = exitErr.ExitCode()
	case waitErr != nil:
		return run, waitErr
	}
	if len(run.unexpected) > 0 {
		return run, fmt.Errorf("hdf %s asked something I wasn't expecting: %q\n%s", strings.Join(args, " "), run.unexpected, run.output())
	}
	return run, nil
}

// watchPrompts copies r to out line by line, without promptMarker, and calls
// answer with the last line of each prompt (the text promptMarker ends). It
// returns once r ends.
func watchPrompts(r io.Reader, out func(text string), answer func(line string)) {
	br := bufio.NewReader(r)
	for {
		text, err := br.ReadString('\n')
		if prompt, ok := strings.CutSuffix(text, promptMarker); ok {
			out(prompt)
			answer(prompt[strings.LastIndexByte(prompt, '\n')+1:])
		} else if text != "" {
			out(text)
		}
		if err != nil {
			return
		}
	}
}

// nodeEnv is the environment hdf runs with on node: the test's own, with HOME
// and the machine branch overridden, and notifications sent to stderr so the
// daemon never pops up (or blocks on) real desktop alerts.
func nodeEnv(node Node) []string {
	env := []string{}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "USERPROFILE=") || strings.HasPrefix(e, "HDF_BRANCH=") {
			continue
		}
		env = append(env, e)
	}
	return append(env, "HOME="+node.home, "USERPROFILE="+node.home, "HDF_BRANCH="+node.branch, "HDF_NOTIFY=log")
}

// fileAnswers answers hdf's per-file prompts by the file each one names: "y"
// for files in accept, "n" for files in skip; acceptRest answers "y" to any
// other per-file prompt. Prompts that name no listed file are unexpected.
func fileAnswers(accept, skip []string, acceptRest bool) responder {
	return func(prompt string) (string, bool) {
		for _, f := range accept {
			if strings.Contains(prompt, f) {
				return "y", true
			}
		}
		for _, f := range skip {
			if strings.Contains(prompt, f) {
				return "n", true
			}
		}
		if acceptRest && strings.Contains(prompt, "~/") {
			return "y", true
		}
		return "", false
	}
}

// orElse tries r first and falls back to next for prompts r doesn't expect.
func (r responder) orElse(next responder) responder {
	return func(prompt string) (string, bool) {
		if a, ok := r(prompt); ok {
			return a, true
		}
		return next(prompt)
	}
}

// answerContaining answers reply to prompts containing substr.
func answerContaining(substr, reply string) responder {
	return func(prompt string) (string, bool) {
		if strings.Contains(prompt, substr) {
			return reply, true
		}
		return "", false
	}
}

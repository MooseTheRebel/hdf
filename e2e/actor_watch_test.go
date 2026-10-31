//go:build e2e

package e2e

import (
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// watchAll runs watchPrompts over what write sends, returning the full output
// and every line it took for a prompt.
func watchAll(write func(w io.Writer)) (string, []string) {
	r, w := io.Pipe()
	go func() {
		write(w)
		_ = w.Close()
	}()
	var (
		mu      sync.Mutex
		out     strings.Builder
		prompts []string
	)
	watchPrompts(r, func(chunk string) {
		mu.Lock()
		out.WriteString(chunk)
		mu.Unlock()
	}, func(line string) bool {
		mu.Lock()
		prompts = append(prompts, line)
		mu.Unlock()
		return true
	})
	return out.String(), prompts
}

func TestWatchPromptsAnswersALineWaitingForInput(t *testing.T) {
	out, prompts := watchAll(func(w io.Writer) {
		_, _ = io.WriteString(w, "Continue promoting? [y/N]: ")
		time.Sleep(3 * promptQuiet)
	})
	if want := []string{"Continue promoting? [y/N]: "}; !slices.Equal(prompts, want) {
		t.Errorf("prompts = %q, want %q", prompts, want)
	}
	if out != "Continue promoting? [y/N]: " {
		t.Errorf("output = %q", out)
	}
}

// A line split by the pipe right after a colon is output still being written,
// not a prompt.
func TestWatchPromptsIgnoresAColonMidLine(t *testing.T) {
	out, prompts := watchAll(func(w io.Writer) {
		_, _ = io.WriteString(w, "Warning: ")
		_, _ = io.WriteString(w, "could not fetch origin\n")
	})
	if len(prompts) != 0 {
		t.Errorf("took %q for prompts", prompts)
	}
	if out != "Warning: could not fetch origin\n" {
		t.Errorf("output = %q", out)
	}
}

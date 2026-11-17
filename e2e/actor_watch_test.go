//go:build e2e

package e2e

import (
	"io"
	"slices"
	"strings"
	"testing"
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
		out     strings.Builder
		prompts []string
	)
	watchPrompts(r, func(text string) {
		out.WriteString(text)
	}, func(line string) {
		prompts = append(prompts, line)
	})
	return out.String(), prompts
}

func TestWatchPromptsAnswersAMarkedPrompt(t *testing.T) {
	out, prompts := watchAll(func(w io.Writer) {
		_, _ = io.WriteString(w, "main has file(s) you haven't pulled:\nContinue promoting? [y/N]: "+promptMarker)
		_, _ = io.WriteString(w, "Promoted.\n")
	})
	if want := []string{"Continue promoting? [y/N]: "}; !slices.Equal(prompts, want) {
		t.Errorf("prompts = %q, want %q", prompts, want)
	}
	if want := "main has file(s) you haven't pulled:\nContinue promoting? [y/N]: Promoted.\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// The marker can arrive split across reads.
func TestWatchPromptsAnswersAPromptWhoseMarkerIsSplit(t *testing.T) {
	_, prompts := watchAll(func(w io.Writer) {
		_, _ = io.WriteString(w, "Choice [1]: "+promptMarker[:3])
		_, _ = io.WriteString(w, promptMarker[3:])
	})
	if want := []string{"Choice [1]: "}; !slices.Equal(prompts, want) {
		t.Errorf("prompts = %q, want %q", prompts, want)
	}
}

// Output that only looks like a prompt isn't one.
func TestWatchPromptsIgnoresUnmarkedLines(t *testing.T) {
	out, prompts := watchAll(func(w io.Writer) {
		_, _ = io.WriteString(w, "Warning: ")
		_, _ = io.WriteString(w, "could not fetch origin\n")
		_, _ = io.WriteString(w, "Last line without newline: ")
	})
	if len(prompts) != 0 {
		t.Errorf("took %q for prompts", prompts)
	}
	if want := "Warning: could not fetch origin\nLast line without newline: "; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

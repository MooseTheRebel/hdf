package notify

import (
	"strings"
	"testing"
)

// TestLogModeWritesInsteadOfNotifying verifies that HDF_NOTIFY=log sends both
// notifiers' output to stderr, labelled by kind, instead of to the OS.
func TestLogModeWritesInsteadOfNotifying(t *testing.T) {
	t.Setenv(ModeEnv, ModeLog)

	var buf strings.Builder

	orig := stderr
	stderr = &buf

	t.Cleanup(func() { stderr = orig })

	if err := Default.Send("hdf", "drift"); err != nil {
		t.Fatalf("Default.Send: %v", err)
	}

	if err := Critical.Send("hdf: push failed", "offline"); err != nil {
		t.Fatalf("Critical.Send: %v", err)
	}

	want := "hdf notification: hdf: drift\nhdf alert: hdf: push failed: offline\n"
	if buf.String() != want {
		t.Errorf("stderr = %q, want %q", buf.String(), want)
	}
}

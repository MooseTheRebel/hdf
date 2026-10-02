package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestLaunchUI(t *testing.T) {
	launchErr := errors.New("plugin failed")
	const (
		vanilla, vuejs         = "vanilla", "vuejs"
		vanillaPath, vuejsPath = "/opt/hdf/hdf-gui-vanilla", "/opt/hdf/hdf-gui-vuejs"
	)
	tests := []struct {
		name       string
		gui        string
		installed  []string
		launchErr  error
		wantLaunch string // path launched, "" for none
		wantErr    string // substring of the error, "" for none
		wantHelp   bool
	}{
		{name: "nothing installed prints help", wantHelp: true},
		{name: "vanilla installed is launched", installed: []string{vanilla}, wantLaunch: vanillaPath},
		{name: "vuejs alone is launched", installed: []string{vuejs}, wantLaunch: vuejsPath},
		{name: "vanilla preferred when both installed", installed: []string{vanilla, vuejs}, wantLaunch: vanillaPath},
		{name: "--gui picks vuejs over vanilla", gui: vuejs, installed: []string{vanilla, vuejs}, wantLaunch: vuejsPath},
		{name: "--gui not installed is an error", gui: vuejs, installed: []string{vanilla}, wantErr: `GUI "vuejs" is not installed (looked for hdf-gui-vuejs`},
		{name: "plugin error is returned", installed: []string{vanilla}, launchErr: launchErr, wantLaunch: vanillaPath, wantErr: "plugin failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := &cobra.Command{Use: "hdf", Long: "hdf help text"}
			cmd.SetOut(&out)
			find := func(name string) (string, bool) {
				for _, n := range tt.installed {
					if n == name {
						return "/opt/hdf/hdf-gui-" + name, true
					}
				}
				return "", false
			}
			launched := ""
			launch := func(path string, args []string) error {
				launched = path
				return tt.launchErr
			}

			err := launchUI(cmd, tt.gui, find, launch)
			if tt.wantErr == "" && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("err = %v, want it to contain %q", err, tt.wantErr)
			}
			if launched != tt.wantLaunch {
				t.Errorf("launched %q, want %q", launched, tt.wantLaunch)
			}
			if got := strings.Contains(out.String(), "hdf help text"); got != tt.wantHelp {
				t.Errorf("help printed = %v, want %v (output %q)", got, tt.wantHelp, out.String())
			}
		})
	}
}

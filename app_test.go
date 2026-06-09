package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsInitialized(t *testing.T) {
	validTOML := `git_push_target = "file:///tmp/bare"
local_dotfiles_dir = "/tmp/repo"
branch = "test-host"
`
	cases := []struct {
		name    string
		setup   func(t *testing.T) string
		wantOk  bool
		wantErr bool
	}{
		{
			name: "missing config — not initialized, no error",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "no-such-config.toml")
			},
			wantOk:  false,
			wantErr: false,
		},
		{
			name: "valid config — initialized, no error",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(p, []byte(validTOML), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantOk:  true,
			wantErr: false,
		},
		{
			name: "corrupted config — not initialized, returns error",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(p, []byte("not valid toml [\x00\x01"), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name: "empty config file — initialized (all fields zero-valued)",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantOk:  true,
			wantErr: false,
		},
		{
			name: "unreadable config file — not initialized, returns error",
			setup: func(t *testing.T) string {
				if os.Getuid() == 0 {
					t.Skip("root bypasses DAC — permission test not meaningful")
				}
				p := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(p, []byte(validTOML), 0o000); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantOk:  false,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.setup(t)
			ok, err := isInitialized(path)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tc.wantOk {
				t.Errorf("isInitialized = %v, want %v", ok, tc.wantOk)
			}
		})
	}
}

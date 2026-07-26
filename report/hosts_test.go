// report/hosts_test.go
package report

import (
	"hdf/repo"
	"os"
	"path/filepath"
	"testing"
)

func TestEnumerateHosts_LocalAndRemoteBranches(t *testing.T) {
	bareDir := t.TempDir()
	if _, _, err := repo.InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}
	bareURL := "file://" + bareDir

	seed, err := repo.Init(t.TempDir())
	if err != nil {
		t.Fatalf("seed Init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seed.Path(), "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CommitFile("seed.txt", "initial"); err != nil {
		t.Fatalf("seed CommitFile: %v", err)
	}
	if err := seed.AddRemote("origin", bareURL); err != nil {
		t.Fatalf("seed AddRemote: %v", err)
	}
	if err := seed.Push("main"); err != nil {
		t.Fatalf("seed Push: %v", err)
	}
	if err := seed.CreateAndCheckoutBranch("host-desktop"); err != nil {
		t.Fatalf("CreateAndCheckoutBranch: %v", err)
	}
	if err := seed.Push("host-desktop"); err != nil {
		t.Fatalf("seed Push host-desktop: %v", err)
	}

	local, err := repo.Clone(bareURL, t.TempDir())
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := local.Fetch(); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if err := local.CheckoutTrackingBranch("host-desktop", "origin"); err != nil {
		t.Fatalf("CheckoutTrackingBranch: %v", err)
	}
	if err := local.CreateAndCheckoutBranch("host-laptop"); err != nil {
		t.Fatalf("CreateAndCheckoutBranch host-laptop: %v", err)
	}

	hosts, err := EnumerateHosts(local, "host-laptop")
	if err != nil {
		t.Fatalf("EnumerateHosts: %v", err)
	}

	byBranch := map[string]HostInfo{}
	for _, h := range hosts {
		byBranch[h.Branch] = h
	}
	if _, ok := byBranch["main"]; ok {
		t.Errorf("hosts = %+v, want no \"main\" entry (not a host-* branch)", hosts)
	}
	laptop, ok := byBranch["host-laptop"]
	if !ok || !laptop.IsCurrent || laptop.SHA == "" {
		t.Errorf("host-laptop entry = %+v, want present, IsCurrent, non-empty SHA", laptop)
	}
	desktop, ok := byBranch["host-desktop"]
	if !ok || desktop.IsCurrent || desktop.SHA == "" {
		t.Errorf("host-desktop entry = %+v, want present, not current, non-empty SHA", desktop)
	}
}

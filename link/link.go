package link

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// HashFile computes the SHA-256 hash of a file and returns "sha256:<hex>".
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

// RepoPathFor returns the path within repoDir that mirrors homePath's position
// relative to $HOME. This prevents name collisions between files with the same
// base name from different directories (e.g. ~/.config/fish/config.fish vs
// ~/.config/nvim/config.fish).
func RepoPathFor(homePath, repoDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return repoPathForHome(homePath, repoDir, home)
}

// repoPathForHome is the testable core of RepoPathFor with an explicit home.
func repoPathForHome(homePath, repoDir, home string) (string, error) {
	rel, err := filepath.Rel(home, homePath)
	if err != nil {
		return "", fmt.Errorf("computing relative path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %s is outside the home directory", homePath)
	}
	return filepath.Join(repoDir, rel), nil
}

// Enroll copies the file at homePath into repoDir (mirroring the directory
// structure relative to $HOME), replaces homePath with a symlink pointing to
// the repo copy, and returns the file's SHA-256 hash.
//
// If symlinking fails after the original has been removed, the file is
// restored from the repo copy before the error is returned.
func Enroll(homePath, repoDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return enrollWithHome(homePath, repoDir, home)
}

// EnrollInHome is Enroll with an explicit homeDir; used by tests and callers
// that already hold the home directory path.
func EnrollInHome(homePath, repoDir, homeDir string) (string, error) {
	return enrollWithHome(homePath, repoDir, homeDir)
}

// RepoPathForHome is RepoPathFor with an explicit homeDir; used by tests and
// callers that already hold the home directory path.
func RepoPathForHome(homePath, repoDir, homeDir string) (string, error) {
	return repoPathForHome(homePath, repoDir, homeDir)
}

func enrollWithHome(homePath, repoDir, home string) (string, error) {
	dst, err := repoPathForHome(homePath, repoDir, home)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("creating repo dirs: %w", err)
	}
	// Already enrolled: homePath is a symlink pointing at dst. Copying would
	// open src and dst as the same file, truncating it before reading.
	if target, err := os.Readlink(homePath); err == nil && target == dst {
		return HashFile(dst)
	}
	if err := copyFile(homePath, dst); err != nil {
		return "", fmt.Errorf("copying to repo: %w", err)
	}
	if err := os.Remove(homePath); err != nil {
		return "", fmt.Errorf("removing original: %w", err)
	}
	if err := os.Symlink(dst, homePath); err != nil {
		// Rollback: restore the original file so the user isn't left without it.
		_ = copyFile(dst, homePath)
		return "", fmt.Errorf("creating symlink: %w", err)
	}
	return HashFile(dst)
}

// Link creates (or re-creates) the symlink at homePath pointing to repoFile.
func Link(homePath, repoFile string) error {
	if _, err := os.Lstat(homePath); err == nil {
		if err := os.Remove(homePath); err != nil {
			return fmt.Errorf("removing existing file: %w", err)
		}
	}
	return os.Symlink(repoFile, homePath)
}

// copyFile copies src to dst, preserving the source file's permission mode.
func copyFile(src, dst string) error {
	finfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, finfo.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

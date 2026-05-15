package link

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// HashFile computes the SHA-256 hash of a file and returns "sha256:<hex>".
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

// Enroll copies the file at homePath into repoDir, replaces homePath with a
// symlink pointing to the repo copy, and returns the file's SHA-256 hash.
func Enroll(homePath, repoDir string) (string, error) {
	dst := filepath.Join(repoDir, filepath.Base(homePath))

	if err := copyFile(homePath, dst); err != nil {
		return "", fmt.Errorf("copying to repo: %w", err)
	}
	if err := os.Remove(homePath); err != nil {
		return "", fmt.Errorf("removing original: %w", err)
	}
	if err := os.Symlink(dst, homePath); err != nil {
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

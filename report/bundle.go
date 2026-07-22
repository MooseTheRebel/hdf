// report/bundle.go
package report

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MaxRepoZipBytes is the compressed-size cap for the packaged backing git
// repo. Reports whose repo exceeds this are refused outright rather than
// silently omitting the repo or truncating it.
const MaxRepoZipBytes = 4 * 1024 * 1024 // 4MB

// ErrRepoTooLarge is returned by CompressRepo when the compressed .git
// directory exceeds MaxRepoZipBytes.
var ErrRepoTooLarge = errors.New("compressed repo exceeds the 4MB report limit")

// CompressRepo archives repoPath's .git directory — which holds every local
// branch, every remote-tracking branch, and HEAD — into an in-memory zip and
// returns its bytes. Returns ErrRepoTooLarge if the result exceeds
// MaxRepoZipBytes.
func CompressRepo(repoPath string) ([]byte, error) {
	gitDir := filepath.Join(repoPath, ".git")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	err := filepath.Walk(gitDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(gitDir, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		// #nosec G122 -- reading from a trusted .git directory
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		if filepath.ToSlash(rel) == "config" {
			data, readErr := io.ReadAll(f)
			if readErr != nil {
				return readErr
			}
			_, err = w.Write(redactGitConfigBytes(data))
			return err
		}

		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("archiving %s: %w", gitDir, err)
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if buf.Len() > MaxRepoZipBytes {
		return nil, ErrRepoTooLarge
	}
	return buf.Bytes(), nil
}

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

// limitWriter wraps a bytes.Buffer and rejects any Write that would grow it
// past limit, returning ErrRepoTooLarge instead. Enforcing the cap at the
// point of write — rather than checking buf.Len() only after the archive is
// fully built — bounds memory use to roughly limit + one internal
// flate-buffer's worth of overshoot, instead of the size of the entire
// (potentially huge) repo being archived.
type limitWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.limit {
		return 0, ErrRepoTooLarge
	}
	return w.buf.Write(p)
}

// CompressRepo archives repoPath's .git directory — which holds every local
// branch, every remote-tracking branch, and HEAD — into an in-memory zip and
// returns its bytes. Returns ErrRepoTooLarge if the result would exceed
// MaxRepoZipBytes; the cap is enforced while writing, so an oversized repo
// is rejected without first being fully buffered into memory.
func CompressRepo(repoPath string) ([]byte, error) {
	gitDir := filepath.Join(repoPath, ".git")
	var buf bytes.Buffer
	zw := zip.NewWriter(&limitWriter{buf: &buf, limit: MaxRepoZipBytes})

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
	// zw.Close() also writes through limitWriter (the zip central directory
	// trailer), so a limit crossed only at Close time still surfaces
	// ErrRepoTooLarge here — no separate post-hoc size check is needed.
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

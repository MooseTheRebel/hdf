package report

import (
	"net/url"
	"strings"
)

// redactURL strips userinfo (username/password or token) from rawURL,
// e.g. "https://user:token@host/repo.git" -> "https://host/repo.git".
// SSH shorthand ("git@host:path") and URLs with no credentials are
// returned unchanged: net/url doesn't parse SSH shorthand as having a
// User component, so there's nothing to strip either way.
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}

// redactGitConfigBytes scans git config file content line-by-line,
// redacting credentials embedded in any "url = ..." value (the form
// go-git's AddRemote writes), so a report never leaks them via the
// bundled .git directory.
func redactGitConfigBytes(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "url") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}
		prefix := line[:idx+1]
		value := strings.TrimSpace(line[idx+1:])
		lines[i] = prefix + " " + redactURL(value)
	}
	return []byte(strings.Join(lines, "\n"))
}

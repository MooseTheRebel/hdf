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
// redacting credentials embedded in any "url" or "pushurl" value (the keys
// go-git's AddRemote and `git remote set-url --push` write), so a report
// never leaks them via the bundled .git directory. Matches the key exactly
// (not by prefix) so it catches "pushurl" without also matching unrelated
// keys that merely start with "url".
func redactGitConfigBytes(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if key != "url" && key != "pushurl" {
			continue
		}
		prefix := line[:idx+1]
		value := strings.TrimSpace(line[idx+1:])
		lines[i] = prefix + " " + redactURL(value)
	}
	return []byte(strings.Join(lines, "\n"))
}

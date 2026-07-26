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

// redactedPlaceholder replaces the value of a credential-bearing git config
// key whose content isn't a URL (so redactURL doesn't apply), such as an
// HTTP header carrying a raw Authorization value.
const redactedPlaceholder = "REDACTED"

// redactGitConfigBytes scans git config file content line-by-line,
// redacting credentials embedded in known credential-bearing keys, so a
// report never leaks them via the bundled .git directory:
//   - "url" / "pushurl" (go-git's AddRemote and `git remote set-url --push`)
//     — redacted via redactURL, which strips only the userinfo component.
//   - "extraheader" (git's http.extraHeader / http.<url>.extraHeader,
//     e.g. `git config http.extraHeader "Authorization: Bearer ..."`) —
//     the value is an arbitrary HTTP header, not a URL, so the whole value
//     is replaced.
//
// Git config keys are case-insensitive, so matching is done on the
// lowercased key. Matches the key exactly (not by prefix) so "pushurl"
// isn't missed by a prefix check for "url", and unrelated keys that merely
// contain "url" or "extraheader" as a substring aren't over-matched.
func redactGitConfigBytes(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		prefix := line[:idx+1]
		switch key {
		case "url", "pushurl":
			value := strings.TrimSpace(line[idx+1:])
			lines[i] = prefix + " " + redactURL(value)
		case "extraheader":
			lines[i] = prefix + " " + redactedPlaceholder
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

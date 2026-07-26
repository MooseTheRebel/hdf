package report

import "testing"

// Shared fixture strings for redaction tests across this package: a
// credentialed remote URL and its expected redacted form.
const (
	testCredentialedURL = "https://user:token@example.com/repo.git" //nolint:gosec // test fixture, not a real credential
	testRedactedURL     = "https://example.com/repo.git"
)

func TestRedactURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "https URL with username and token is redacted",
			in:   testCredentialedURL,
			want: testRedactedURL,
		},
		{
			name: "https URL with no credentials is unchanged",
			in:   testRedactedURL,
			want: testRedactedURL,
		},
		{
			name: "ssh shorthand is unchanged (git@ is not parsed as userinfo)",
			in:   "git@github.com:user/repo.git",
			want: "git@github.com:user/repo.git",
		},
		{
			name: "empty string is unchanged",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactURL(tc.in)
			if got != tc.want {
				t.Errorf("redactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedactGitConfigBytes(t *testing.T) {
	in := []byte("[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + testCredentialedURL + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n")
	got := string(redactGitConfigBytes(in))
	if contains(got, testCredentialedURL) {
		t.Errorf("redactGitConfigBytes output still contains credentials:\n%s", got)
	}
	if !contains(got, testRedactedURL) {
		t.Errorf("redactGitConfigBytes output missing redacted URL:\n%s", got)
	}
	if !contains(got, `bare = false`) {
		t.Errorf("redactGitConfigBytes should leave unrelated lines untouched:\n%s", got)
	}
}

// TestRedactGitConfigBytes_RedactsPushURL verifies pushurl entries (written
// by e.g. `git remote set-url --push origin <url>`) are redacted too, not
// just url — a separate credential-bearing key that a plain "url" prefix
// match would miss.
func TestRedactGitConfigBytes_RedactsPushURL(t *testing.T) {
	const pushCredentialedURL = "https://pushuser:pushtoken@example.com/other-repo.git" //nolint:gosec // test fixture, not a real credential
	const pushRedactedURL = "https://example.com/other-repo.git"

	in := []byte("[remote \"origin\"]\n\turl = " + testCredentialedURL + "\n\tpushurl = " + pushCredentialedURL + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n")
	got := string(redactGitConfigBytes(in))
	if contains(got, "pushuser") || contains(got, "pushtoken") {
		t.Errorf("redactGitConfigBytes output still contains pushurl credentials:\n%s", got)
	}
	if !contains(got, pushRedactedURL) {
		t.Errorf("redactGitConfigBytes output missing redacted pushurl:\n%s", got)
	}
	if contains(got, testCredentialedURL) {
		t.Errorf("redactGitConfigBytes output still contains url credentials:\n%s", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

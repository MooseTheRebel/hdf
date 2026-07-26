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

// TestRedactGitConfigBytes_RedactsHTTPExtraHeader verifies both the global
// http.extraHeader and URL-scoped http.<url>.extraHeader forms are
// redacted. Git uses this key to inject arbitrary HTTP headers — including
// "Authorization: Basic/Bearer ..." — on every request, so its value can
// carry a raw credential that isn't a URL and so can't be run through
// redactURL; the whole value is replaced instead.
func TestRedactGitConfigBytes_RedactsHTTPExtraHeader(t *testing.T) {
	in := []byte("[http]\n\textraHeader = Authorization: Basic dXNlcjpwYXNz\n" +
		"[http \"https://example.com/\"]\n\textraheader = Authorization: Bearer abc123token\n" +
		"[core]\n\tbare = false\n")
	got := string(redactGitConfigBytes(in))
	if contains(got, "dXNlcjpwYXNz") {
		t.Errorf("redactGitConfigBytes output still contains global extraHeader credentials:\n%s", got)
	}
	if contains(got, "abc123token") {
		t.Errorf("redactGitConfigBytes output still contains URL-scoped extraHeader credentials:\n%s", got)
	}
	if !contains(got, "bare = false") {
		t.Errorf("redactGitConfigBytes should leave unrelated lines untouched:\n%s", got)
	}
}

// TestRedactGitConfigBytes_KeyMatchIsCaseInsensitive verifies uppercase or
// mixed-case key spellings (git config keys are case-insensitive) are still
// matched, so an oddly-cased key can't bypass redaction.
func TestRedactGitConfigBytes_KeyMatchIsCaseInsensitive(t *testing.T) {
	in := []byte("[remote \"origin\"]\n\tURL = " + testCredentialedURL + "\n")
	got := string(redactGitConfigBytes(in))
	if contains(got, testCredentialedURL) {
		t.Errorf("redactGitConfigBytes output still contains credentials for uppercase key:\n%s", got)
	}
	if !contains(got, testRedactedURL) {
		t.Errorf("redactGitConfigBytes output missing redacted URL for uppercase key:\n%s", got)
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

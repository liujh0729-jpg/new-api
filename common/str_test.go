package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactURLCredentials(t *testing.T) {
	leaky := `fatal: unable to access 'https://x-access-token:not-a-prefixed-secret@github.com/acme/new-api.git/': 403`
	redacted := RedactURLCredentials(leaky)
	require.NotContains(t, redacted, "not-a-prefixed-secret")
	require.NotContains(t, redacted, "x-access-token")
	require.Contains(t, redacted, "https://***@github.com/acme/new-api.git/")

	// Git also prints remotes that authenticate with a username only.
	require.Equal(t, "https://***@gitee.com/repo", RedactURLCredentials("https://deploy@gitee.com/repo"))

	// Hosts and paths without userinfo must survive untouched, including an "@"
	// that appears after the host.
	for _, plain := range []string{
		"https://github.com/acme/new-api",
		"https://github.com/acme/new-api/commits/abc@def",
		"mailto://ops@example.com",
	} {
		require.Equal(t, plain, RedactURLCredentials(plain), plain)
	}
}

func TestRedactSecretsCatchesKeyShapes(t *testing.T) {
	text := `Authorization: Bearer sk-abcdefghijklmnopqrstuvwxyz012345 and api_key=second-secret-value-here`
	redacted := RedactSecrets(text)
	require.NotContains(t, redacted, "sk-abcdefghijklmnopqrstuvwxyz012345")
	require.NotContains(t, redacted, "second-secret-value-here")

	// A literal secret is stripped even when it carries no credential shape.
	require.Equal(t, "job [redacted] failed", RedactSecrets("job sup3rjob failed", "sup3rjob"))
}

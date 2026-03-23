package cmd

import (
	"fmt"
	"os"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/provider"
)

var host provider.Host

func requireProvider() error {
	if host != nil {
		return nil
	}
	h, err := detectProvider("origin")
	if err != nil {
		return err
	}
	host = h
	return nil
}

func detectProvider(remote string) (provider.Host, error) {
	remoteURL, err := git.RemoteURL(remote)
	if err != nil {
		return nil, fmt.Errorf("could not read remote %q: %w", remote, err)
	}

	info, err := provider.ParseRemoteURL(remoteURL)
	if err != nil {
		return nil, err
	}

	if info.IsGitHub() {
		return provider.NewGitHub()
	}

	// Non-GitHub remote: try Forgejo
	token := forgejoToken(info.Host)
	if token == "" {
		return nil, fmt.Errorf("no API token for %s (set FORGEJO_TOKEN or ST_TOKEN_%s)", info.Host, sanitizeEnvKey(info.Host))
	}

	instanceURL := "https://" + info.Host
	return provider.NewForgejo(instanceURL, token, info.Owner, info.Repo)
}

// forgejoToken returns an API token for the given host.
// Checks host-specific env var first, then generic FORGEJO_TOKEN.
func forgejoToken(host string) string {
	if t := os.Getenv("ST_TOKEN_" + sanitizeEnvKey(host)); t != "" {
		return t
	}
	return os.Getenv("FORGEJO_TOKEN")
}

// sanitizeEnvKey converts a hostname to a valid env var suffix.
// e.g. "codeberg.org" -> "CODEBERG_ORG"
func sanitizeEnvKey(s string) string {
	var out []byte
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z':
			out = append(out, c-32) // uppercase
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

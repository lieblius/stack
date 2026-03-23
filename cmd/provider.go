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
	token := os.Getenv("FORGEJO_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("no API token for %s (set FORGEJO_TOKEN)", info.Host)
	}

	return provider.NewForgejo(info.BaseURL, token, info.Owner, info.Repo)
}

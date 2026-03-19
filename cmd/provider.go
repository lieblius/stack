package cmd

import "github.com/liebl/stack/internal/provider"

var host *provider.GitHub

func requireProvider() error {
	if host != nil {
		return nil
	}
	p, err := provider.NewGitHub()
	if err != nil {
		return err
	}
	host = p
	return nil
}

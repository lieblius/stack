package provider

import (
	"fmt"
	"net/url"
	"strings"
)

// RemoteInfo holds the parsed components of a git remote URL.
type RemoteInfo struct {
	Host    string // e.g. "github.com", "localhost:3333"
	BaseURL string // e.g. "https://github.com", "http://localhost:3333"
	Owner   string
	Repo    string
}

// ParseRemoteURL extracts host, owner, and repo from a git remote URL.
// Supports both SSH (git@host:owner/repo.git) and HTTPS (https://host/owner/repo.git) formats.
func ParseRemoteURL(rawURL string) (*RemoteInfo, error) {
	// SSH format: git@host:owner/repo.git
	if strings.Contains(rawURL, "@") && strings.Contains(rawURL, ":") && !strings.Contains(rawURL, "://") {
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("cannot parse SSH remote URL: %s", rawURL)
		}
		hostPart := parts[0]
		if idx := strings.Index(hostPart, "@"); idx >= 0 {
			hostPart = hostPart[idx+1:]
		}
		path := strings.TrimSuffix(parts[1], ".git")
		ownerRepo := strings.SplitN(path, "/", 2)
		if len(ownerRepo) != 2 {
			return nil, fmt.Errorf("cannot parse owner/repo from SSH remote: %s", rawURL)
		}
		return &RemoteInfo{
			Host:    hostPart,
			BaseURL: "https://" + hostPart,
			Owner:   ownerRepo[0],
			Repo:    ownerRepo[1],
		}, nil
	}

	// HTTPS format
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse remote URL: %s: %w", rawURL, err)
	}
	path := strings.TrimPrefix(strings.TrimSuffix(u.Path, ".git"), "/")
	ownerRepo := strings.SplitN(path, "/", 2)
	if len(ownerRepo) != 2 || ownerRepo[0] == "" || ownerRepo[1] == "" {
		return nil, fmt.Errorf("cannot parse owner/repo from URL: %s", rawURL)
	}
	return &RemoteInfo{
		Host:    u.Host,
		BaseURL: u.Scheme + "://" + u.Host,
		Owner:   ownerRepo[0],
		Repo:    ownerRepo[1],
	}, nil
}

func (r *RemoteInfo) IsGitHub() bool {
	return strings.Contains(r.Host, "github.com")
}

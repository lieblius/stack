package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GitHub implements provider operations using the gh CLI.
type GitHub struct{}

// NewGitHub creates a GitHub provider, verifying that the gh CLI is available.
func NewGitHub() (*GitHub, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found in PATH; install from https://cli.github.com")
	}
	return &GitHub{}, nil
}

// ghPR is the JSON shape returned by gh pr view --json.
type ghPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	URL         string `json:"url"`
	Body        string `json:"body"`
}

func (p *ghPR) toPullRequest() *PullRequest {
	var state PRState
	switch p.State {
	case "OPEN":
		state = PROpen
	case "MERGED":
		state = PRMerged
	case "CLOSED":
		state = PRClosed
	default:
		state = PRState(strings.ToLower(p.State))
	}
	return &PullRequest{
		Number: p.Number,
		Title:  p.Title,
		State:  state,
		Head:   p.HeadRefName,
		Base:   p.BaseRefName,
		URL:    p.URL,
		Body:   p.Body,
	}
}

func (g *GitHub) PRForBranch(branch string) (*PullRequest, error) {
	cmd := exec.Command("gh", "pr", "view", branch,
		"--json", "number,title,state,headRefName,baseRefName,url,body")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "no pull requests found") {
			return nil, nil
		}
		return nil, fmt.Errorf("gh pr view %s: %s", branch, strings.TrimSpace(stderr.String()))
	}
	var pr ghPR
	if err := json.Unmarshal(stdout.Bytes(), &pr); err != nil {
		return nil, fmt.Errorf("parsing PR info for %s: %w", branch, err)
	}
	return pr.toPullRequest(), nil
}

func (g *GitHub) CreatePR(head, base, title, body string) (*PullRequest, error) {
	cmd := exec.Command("gh", "pr", "create",
		"--head", head, "--base", base,
		"--title", title, "--body", body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh pr create: %s", strings.TrimSpace(stderr.String()))
	}
	return g.PRForBranch(head)
}

func (g *GitHub) EditPRBase(number int, newBase string) error {
	cmd := exec.Command("gh", "pr", "edit", fmt.Sprintf("%d", number),
		"--base", newBase)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr edit %d --base %s: %s", number, newBase, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (g *GitHub) EditPRBody(number int, body string) error {
	tmpFile, err := os.CreateTemp("", "st-pr-body-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	tmpFile.Close()

	cmd := exec.Command("gh", "pr", "edit", fmt.Sprintf("%d", number),
		"--body-file", tmpFile.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr edit %d body: %s", number, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (g *GitHub) MergePR(number int, strategy string, interactive bool) error {
	if interactive {
		args := []string{"pr", "merge", fmt.Sprintf("%d", number)}
		switch strategy {
		case "squash":
			args = append(args, "--squash")
		case "merge":
			args = append(args, "--merge")
		case "rebase":
			args = append(args, "--rebase")
		}
		cmd := exec.Command("gh", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	method := "squash"
	switch strategy {
	case "merge":
		method = "merge"
	case "rebase":
		method = "rebase"
	}

	cmd := exec.Command("gh", "api", "--method", "PUT",
		fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/merge", number),
		"-f", "merge_method="+method)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		return fmt.Errorf("merging PR #%d: %s", number, errMsg)
	}
	return nil
}

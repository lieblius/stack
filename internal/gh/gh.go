package gh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type PRInfo struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	URL         string `json:"url"`
}

func EnsureInstalled() error {
	_, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("gh CLI not found in PATH; install from https://cli.github.com")
	}
	return nil
}

func PRForBranch(branch string) (*PRInfo, error) {
	cmd := exec.Command("gh", "pr", "view", branch,
		"--json", "number,title,state,headRefName,baseRefName,url")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "no pull requests found") {
			return nil, nil
		}
		return nil, fmt.Errorf("gh pr view %s: %s", branch, strings.TrimSpace(errMsg))
	}
	var pr PRInfo
	if err := json.Unmarshal(stdout.Bytes(), &pr); err != nil {
		return nil, fmt.Errorf("parsing PR info for %s: %w", branch, err)
	}
	return &pr, nil
}

func EditPRBase(prNumber int, newBase string) error {
	cmd := exec.Command("gh", "pr", "edit", fmt.Sprintf("%d", prNumber),
		"--base", newBase)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr edit %d --base %s: %s", prNumber, newBase, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func CreatePR(head, base, title, body string) (*PRInfo, error) {
	cmd := exec.Command("gh", "pr", "create",
		"--head", head, "--base", base,
		"--title", title, "--body", body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh pr create: %s", strings.TrimSpace(stderr.String()))
	}
	// gh pr create outputs the URL; fetch full info
	return PRForBranch(head)
}

func MergePR(prNumber int, strategy string) error {
	args := []string{"pr", "merge", fmt.Sprintf("%d", prNumber)}
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

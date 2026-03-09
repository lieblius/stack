package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func CurrentBranch() (string, error) {
	return Run("rev-parse", "--abbrev-ref", "HEAD")
}

func MergeBase(a, b string) (string, error) {
	return Run("merge-base", a, b)
}

func BranchExists(name string) bool {
	_, err := Run("rev-parse", "--verify", "refs/heads/"+name)
	return err == nil
}

func SHA(ref string) (string, error) {
	return Run("rev-parse", ref)
}

func ShortSHA(ref string) (string, error) {
	return Run("rev-parse", "--short", ref)
}

func Checkout(branch string) error {
	_, err := Run("checkout", branch)
	return err
}

func Pull(remote, branch string) error {
	_, err := Run("pull", remote, branch)
	return err
}

func RebaseOnto(onto, upstream, branch string) error {
	_, err := Run("rebase", "--onto", onto, upstream, branch)
	return err
}

func ForcePush(remote, branch string) error {
	_, err := Run("push", "--force-with-lease", remote, branch)
	return err
}

func RevList(rangeSpec string) ([]string, error) {
	out, err := Run("rev-list", "--oneline", rangeSpec)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func RemoteBranchExists(remote, branch string) bool {
	out, err := Run("ls-remote", "--heads", remote, "refs/heads/"+branch)
	return err == nil && out != ""
}

func FirstCommitSubject(rangeSpec string) string {
	out, err := Run("log", "--format=%s", "-1", rangeSpec)
	if err != nil {
		return ""
	}
	return out
}

func DeleteRemoteBranch(remote, branch string) error {
	_, err := Run("push", remote, "--delete", branch)
	return err
}

func DeleteLocalBranch(branch string) error {
	_, err := Run("branch", "-D", branch)
	return err
}

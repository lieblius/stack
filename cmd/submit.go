package cmd

import (
	"fmt"
	"strings"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/liebl/stack/internal/provider"
	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Push all branches and create/update PRs",
	Long: `Push all tracked branches and ensure each has a PR with the correct
base branch. Creates new PRs where needed, updates base branches where wrong.
Updates stack navigation in each PR body.`,
	RunE: runSubmit,
}

var (
	submitRemote string
	submitDryRun bool
)

func init() {
	submitCmd.Flags().StringVar(&submitRemote, "remote", "origin", "remote name")
	submitCmd.Flags().BoolVar(&submitDryRun, "dry-run", false, "show what would happen without doing it")
}

type prEntry struct {
	branch string
	pr     *provider.PullRequest
}

func runSubmit(cmd *cobra.Command, args []string) error {
	if err := requireProvider(); err != nil {
		return err
	}

	pruned, err := meta.PruneStale()
	if err != nil {
		return err
	}
	for _, name := range pruned {
		fmt.Printf("  pruned stale branch: %s\n", name)
	}

	all, err := meta.AllTracked()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("no tracked branches")
	}

	var entries []prEntry

	for _, m := range all {
		commits, _ := git.RevList(m.Parent + ".." + m.Name)
		if len(commits) == 0 {
			fmt.Printf("  %s: skipping (no commits above %s)\n", m.Name, m.Parent)
			continue
		}

		// Push
		fmt.Printf("  %s: pushing...\n", m.Name)
		if !submitDryRun {
			if err := git.ForcePush(submitRemote, m.Name); err != nil {
				return fmt.Errorf("pushing %s: %w", m.Name, err)
			}
		}

		// Check existing PR
		pr, err := host.PRForBranch(m.Name)
		if err != nil {
			fmt.Printf("  %s: warning: could not check PR: %v\n", m.Name, err)
		}

		if pr == nil {
			// Create PR
			title := git.FirstCommitSubject(m.Parent + ".." + m.Name)
			if title == "" {
				title = m.Name
			}
			fmt.Printf("  %s: creating PR (base: %s, title: %q)...\n", m.Name, m.Parent, title)
			if !submitDryRun {
				newPR, err := host.CreatePR(m.Name, m.Parent, title, "")
				if err != nil {
					return fmt.Errorf("creating PR for %s: %w", m.Name, err)
				}
				fmt.Printf("  %s: created PR #%d (%s)\n", m.Name, newPR.Number, newPR.URL)
				pr = newPR
			}
		} else if pr.Base != m.Parent {
			// PR exists but wrong base
			fmt.Printf("  %s: updating PR #%d base: %s -> %s\n", m.Name, pr.Number, pr.Base, m.Parent)
			if !submitDryRun {
				if err := host.EditPRBase(pr.Number, m.Parent); err != nil {
					return fmt.Errorf("updating PR base for %s: %w", m.Name, err)
				}
			}
		} else {
			fmt.Printf("  %s: PR #%d up to date\n", m.Name, pr.Number)
		}

		if pr != nil {
			entries = append(entries, prEntry{m.Name, pr})
		}
	}

	// Update stack navigation in PR bodies
	if !submitDryRun && len(entries) > 1 {
		fmt.Println("\n  Updating stack navigation...")
		for _, e := range entries {
			nav := buildStackNav(entries, e.branch)
			newBody := spliceStackNav(e.pr.Body, nav)
			if newBody != e.pr.Body {
				if err := host.EditPRBody(e.pr.Number, newBody); err != nil {
					fmt.Printf("  warning: could not update nav for PR #%d: %v\n", e.pr.Number, err)
				}
			}
		}
	}

	fmt.Println("\nSubmit complete.")
	return nil
}

const (
	navStart = "<!-- st:stack -->"
	navEnd   = "<!-- /st:stack -->"
)

func buildStackNav(entries []prEntry, currentBranch string) string {
	var b strings.Builder
	b.WriteString(navStart + "\n")
	b.WriteString("**Stack:**\n")
	for i, e := range entries {
		if e.branch == currentBranch {
			b.WriteString(fmt.Sprintf("%d. **#%d %s**\n", i+1, e.pr.Number, e.branch))
		} else {
			b.WriteString(fmt.Sprintf("%d. #%d %s\n", i+1, e.pr.Number, e.branch))
		}
	}
	b.WriteString(navEnd)
	return b.String()
}

func spliceStackNav(body, nav string) string {
	startIdx := strings.Index(body, navStart)
	endIdx := strings.Index(body, navEnd)
	if startIdx >= 0 && endIdx >= 0 {
		return body[:startIdx] + nav + body[endIdx+len(navEnd):]
	}
	if strings.TrimSpace(body) == "" {
		return nav
	}
	return body + "\n\n" + nav
}

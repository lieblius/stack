package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/gh"
	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Push all branches and create/update PRs",
	Long: `Push all tracked branches and ensure each has a PR with the correct
base branch. Creates new PRs where needed, updates base branches where wrong.`,
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

func runSubmit(cmd *cobra.Command, args []string) error {
	if err := gh.EnsureInstalled(); err != nil {
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
		pr, err := gh.PRForBranch(m.Name)
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
				newPR, err := gh.CreatePR(m.Name, m.Parent, title, "")
				if err != nil {
					return fmt.Errorf("creating PR for %s: %w", m.Name, err)
				}
				fmt.Printf("  %s: created PR #%d (%s)\n", m.Name, newPR.Number, newPR.URL)
			}
		} else if pr.BaseRefName != m.Parent {
			// PR exists but wrong base
			fmt.Printf("  %s: updating PR #%d base: %s -> %s\n", m.Name, pr.Number, pr.BaseRefName, m.Parent)
			if !submitDryRun {
				if err := gh.EditPRBase(pr.Number, m.Parent); err != nil {
					return fmt.Errorf("updating PR base for %s: %w", m.Name, err)
				}
			}
		} else {
			fmt.Printf("  %s: PR #%d up to date\n", m.Name, pr.Number)
		}
	}

	fmt.Println("\nSubmit complete.")
	return nil
}

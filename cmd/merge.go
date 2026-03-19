package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/liebl/stack/internal/provider"
	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Squash merge the bottom PR and sync the stack",
	Long: `Merge the bottom open PR in the stack via squash, repoint child branches
to trunk, rebase the remaining stack, and clean up the merged branch.`,
	RunE: runMerge,
}

var (
	mergeTrunk    string
	mergeRemote   string
	mergeDryRun   bool
	mergeYes      bool
	mergeAll      bool
	mergeCI       bool
	mergeStrategy string
)

func init() {
	mergeCmd.Flags().StringVar(&mergeTrunk, "trunk", "main", "trunk branch name")
	mergeCmd.Flags().StringVar(&mergeRemote, "remote", "origin", "remote name")
	mergeCmd.Flags().BoolVar(&mergeDryRun, "dry-run", false, "show what would happen without doing it")
	mergeCmd.Flags().BoolVar(&mergeYes, "yes", false, "skip confirmation prompt")
	mergeCmd.Flags().BoolVar(&mergeAll, "all", false, "merge all PRs in the stack one by one")
	mergeCmd.Flags().BoolVar(&mergeCI, "ci", false, "skip confirmation prompts (same as --yes)")
	mergeCmd.Flags().StringVar(&mergeStrategy, "strategy", "squash", "merge strategy: squash, merge, rebase")
}

func runMerge(cmd *cobra.Command, args []string) error {
	strategy := provider.MergeStrategy(mergeStrategy)
	switch strategy {
	case provider.MergeSquash, provider.MergeMerge, provider.MergeRebase:
	default:
		return fmt.Errorf("unknown merge strategy: %q (valid: squash, merge, rebase)", mergeStrategy)
	}

	if err := requireProvider(); err != nil {
		return err
	}

	if err := recoverFromContinue(); err != nil {
		return err
	}

	pruned, err := meta.PruneStale()
	if err != nil {
		return err
	}
	for _, name := range pruned {
		fmt.Printf("  pruned stale branch: %s\n", name)
	}

	if mergeAll {
		all, err := meta.AllTracked()
		if err != nil {
			return err
		}

		// List all open PRs in topo order for upfront confirmation
		var targets []string
		for _, m := range all {
			pr, _ := host.PRForBranch(m.Name)
			if pr != nil && pr.State == provider.PROpen {
				targets = append(targets, fmt.Sprintf("  PR #%d (%s)", pr.Number, m.Name))
			}
		}
		if len(targets) == 0 {
			return fmt.Errorf("no open PRs found in the stack")
		}

		if !mergeYes && !mergeCI && !mergeDryRun {
			fmt.Printf("Will merge %d PRs into %s (bottom to top):\n", len(targets), mergeTrunk)
			for _, t := range targets {
				fmt.Println(t)
			}
			if err := confirmAction("Proceed?"); err != nil {
				return err
			}
		}

		count := 0
		for {
			merged, err := mergeOne()
			if err != nil {
				return err
			}
			if !merged {
				break
			}
			count++
			fmt.Println()
		}
		fmt.Printf("Merged %d PRs.\n", count)
		return nil
	}

	merged, err := mergeOne()
	if err != nil {
		return err
	}
	if !merged {
		return fmt.Errorf("no open PR found at the bottom of the stack (parent must be %s)", mergeTrunk)
	}
	return nil
}

// mergeOne finds and merges the bottom open PR. Returns false if none found.
func mergeOne() (bool, error) {
	all, err := meta.AllTracked()
	if err != nil {
		return false, err
	}
	if len(all) == 0 {
		return false, nil
	}

	// Find bottom open PR: first in topo order whose parent is trunk
	var target meta.BranchMeta
	var targetPR *provider.PullRequest
	for _, m := range all {
		if m.Parent != mergeTrunk {
			continue
		}
		pr, _ := host.PRForBranch(m.Name)
		if pr != nil && pr.State == provider.PROpen {
			target = m
			targetPR = pr
			break
		}
	}

	if targetPR == nil {
		return false, nil
	}

	// Confirm (skip if --all since gh prompts interactively for each merge)
	if !mergeYes && !mergeAll && !mergeCI && !mergeDryRun {
		if err := confirmAction(fmt.Sprintf("Merge PR #%d (%s) into %s?", targetPR.Number, target.Name, mergeTrunk)); err != nil {
			return false, err
		}
	}

	// Ensure clean tree before any local operations
	if !mergeDryRun {
		if err := ensureCleanTree(); err != nil {
			return false, err
		}
	}

	origBranch, _ := git.CurrentBranch()

	var swState *git.SkipWorktreeState
	if !mergeDryRun {
		swState, err = git.SaveSkipWorktree()
		if err != nil {
			return false, fmt.Errorf("saving skip-worktree state: %w", err)
		}
		if len(swState.Files()) > 0 {
			fmt.Printf("Saved %d skip-worktree files\n", len(swState.Files()))
		}
	}

	conflicted := false
	defer func() {
		if !conflicted && swState != nil {
			swState.Restore()
		}
		restoreCheckout(origBranch, mergeDryRun)
	}()

	// Repoint children FIRST (key invariant: never delete a branch that is the base of another open PR)
	children := meta.Children(target.Name, all)
	for _, child := range children {
		childPR, _ := host.PRForBranch(child)
		if childPR != nil && childPR.State == provider.PROpen {
			fmt.Printf("  repointing PR #%d (%s) base: %s -> %s\n", childPR.Number, child, target.Name, mergeTrunk)
			if !mergeDryRun {
				if err := host.EditPRBase(childPR.Number, mergeTrunk); err != nil {
					return false, fmt.Errorf("repointing PR for %s: %w", child, err)
				}
			}
		}
		fmt.Printf("  reparenting %s: %s -> %s\n", child, target.Name, mergeTrunk)
		if !mergeDryRun {
			if err := meta.SetParent(child, mergeTrunk); err != nil {
				return false, fmt.Errorf("reparenting %s: %w", child, err)
			}
		}
	}

	// Merge
	fmt.Printf("\nMerging PR #%d (%s) via squash...\n", targetPR.Number, target.Name)
	if !mergeDryRun {
		if err := host.MergePR(targetPR.Number, provider.MergeStrategy(mergeStrategy)); err != nil {
			return false, fmt.Errorf("merging PR #%d: %w", targetPR.Number, err)
		}
	}

	// Pull trunk
	fmt.Printf("Pulling %s/%s...\n", mergeRemote, mergeTrunk)
	if !mergeDryRun {
		if err := git.Checkout(mergeTrunk); err != nil {
			return false, fmt.Errorf("checking out %s: %w", mergeTrunk, err)
		}
		if err := git.Pull(mergeRemote, mergeTrunk); err != nil {
			return false, fmt.Errorf("pulling %s: %w", mergeTrunk, err)
		}
	}

	// Sync remaining stack
	var alive []meta.BranchMeta
	if mergeDryRun {
		// In dry-run, metadata wasn't updated; construct alive list with simulated reparenting
		childSet := make(map[string]bool)
		for _, child := range children {
			childSet[child] = true
		}
		for _, m := range all {
			if m.Name == target.Name {
				continue
			}
			if childSet[m.Name] {
				m.Parent = mergeTrunk
			}
			alive = append(alive, m)
		}
	} else {
		all, err = meta.AllTracked()
		if err != nil {
			fmt.Printf("Warning: could not reload tracked branches: %v\n", err)
			fmt.Println("Run 'st sync' to complete the rebase.")
		}
		for _, m := range all {
			if m.Name != target.Name {
				alive = append(alive, m)
			}
		}
	}

	if len(alive) > 0 {
		result, err := rebaseTrackedBranches(alive, mergeRemote, mergeDryRun)
		if err != nil {
			if isConflictError(err) {
				conflicted = true
			}
			// Merge already happened (irreversible), so warn but don't fail
			fmt.Printf("\nMerge succeeded but sync failed: %v\n", err)
			fmt.Println("Run 'st sync' to complete the rebase.")
			if conflicted {
				return false, err
			}
		} else if result.Rebased > 0 {
			fmt.Printf("\n%d branches rebased.\n", result.Rebased)
		}
	}

	// Cleanup merged branch
	fmt.Printf("\nCleaning up %s...\n", target.Name)
	if !mergeDryRun {
		meta.Delete(target.Name)
		git.DeleteLocalBranch(target.Name)
		if err := git.DeleteRemoteBranch(mergeRemote, target.Name); err != nil {
			fmt.Printf("  warning: could not delete remote branch %s: %v\n", target.Name, err)
		}
	}

	fmt.Printf("\nMerge complete. PR #%d (%s) merged into %s.\n", targetPR.Number, target.Name, mergeTrunk)
	return true, nil
}

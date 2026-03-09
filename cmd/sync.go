package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/gh"
	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Rebase stack after squash merges land on trunk",
	Long: `Pull latest trunk, detect merged branches, rebase the remaining
stack, force push, and update PR base branches on GitHub.`,
	RunE: runSync,
}

var (
	syncTrunk  string
	syncRemote string
	syncDryRun bool
)

func init() {
	syncCmd.Flags().StringVar(&syncTrunk, "trunk", "main", "trunk branch name")
	syncCmd.Flags().StringVar(&syncRemote, "remote", "origin", "remote name")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "show what would happen without doing it")
}

func runSync(cmd *cobra.Command, args []string) error {
	// Check for interrupted rebase from a previous sync
	if err := recoverFromContinue(); err != nil {
		return err
	}

	all, err := meta.AllTracked()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("no tracked branches")
	}

	// Verify clean working tree before we start
	if !syncDryRun {
		if err := ensureCleanTree(); err != nil {
			return err
		}
	}

	origBranch, _ := git.CurrentBranch()

	// Step 1: Save and clear skip-worktree BEFORE checkout/pull
	// so the checkout doesn't interact with custom skip-worktree files
	var swState *git.SkipWorktreeState
	if !syncDryRun {
		swState, err = git.SaveSkipWorktree()
		if err != nil {
			return fmt.Errorf("saving skip-worktree state: %w", err)
		}
		if len(swState.Files()) > 0 {
			fmt.Printf("Saved %d skip-worktree files\n", len(swState.Files()))
		}
	}

	// restoreSkipWorktree is called explicitly on success, NOT via defer,
	// because on conflict we want to leave bits cleared so git rebase --continue works.
	conflicted := false
	defer func() {
		if !conflicted && swState != nil {
			swState.Restore()
		}
	}()

	// Step 2: Pull trunk
	fmt.Printf("Pulling %s/%s...\n", syncRemote, syncTrunk)
	if !syncDryRun {
		if err := git.Checkout(syncTrunk); err != nil {
			return fmt.Errorf("checking out %s: %w", syncTrunk, err)
		}
		if err := git.Pull(syncRemote, syncTrunk); err != nil {
			return fmt.Errorf("pulling %s: %w", syncTrunk, err)
		}
	}

	// Step 3: Identify merged branches and reparent their children
	var merged []meta.BranchMeta
	var alive []meta.BranchMeta
	for _, m := range all {
		pr, _ := gh.PRForBranch(m.Name)
		if pr != nil && pr.State == "MERGED" {
			merged = append(merged, m)
			fmt.Printf("  %s: merged (PR #%d)\n", m.Name, pr.Number)
		} else {
			alive = append(alive, m)
		}
	}

	mergedSet := make(map[string]bool)
	for _, m := range merged {
		mergedSet[m.Name] = true
	}

	for i := range alive {
		m := &alive[i]
		if mergedSet[m.Parent] {
			newParent := m.Parent
			for mergedSet[newParent] {
				pm, _ := meta.Get(newParent)
				if pm == nil {
					newParent = syncTrunk
					break
				}
				newParent = pm.Parent
			}
			fmt.Printf("  %s: reparenting %s -> %s\n", m.Name, m.Parent, newParent)
			m.Parent = newParent
		}
	}

	// Step 4: Walk alive branches in topo order, rebase any whose base != parent tip.
	// Check inline (not pre-computed) so cascade propagates naturally.
	rebased := 0
	simulatedTips := make(map[string]string) // dry-run only

	for i, m := range alive {
		parentSHA, err := git.SHA(m.Parent)
		if err != nil {
			return fmt.Errorf("resolving parent %s: %w", m.Parent, err)
		}

		if sim, ok := simulatedTips[m.Parent]; ok {
			parentSHA = sim
		}

		if m.Base == parentSHA {
			continue
		}

		fmt.Printf("\nRebasing %s onto %s (base: %s)...\n", m.Name, m.Parent, m.Base[:7])

		if syncDryRun {
			commits, _ := git.RevList(m.Base + ".." + m.Name)
			fmt.Printf("  would replay %d commits\n", len(commits))
			simulatedTips[m.Name] = "simulated-" + m.Name
			alive[i].Base = parentSHA
			rebased++
			continue
		}

		// Save continue state BEFORE attempting rebase
		meta.SaveContinueState(meta.ContinueState{
			Branch:    m.Name,
			Parent:    m.Parent,
			ParentSHA: parentSHA,
		})

		if err := git.RebaseOnto(m.Parent, m.Base, m.Name); err != nil {
			conflicted = true // prevent skip-worktree restore
			fmt.Printf("\nConflict rebasing %s. To resolve:\n", m.Name)
			fmt.Printf("  1. Fix conflicts and run: git rebase --continue\n")
			fmt.Printf("  2. Then run: st sync\n")
			fmt.Printf("  Or abort with: git rebase --abort && st sync\n")
			return fmt.Errorf("rebase conflict on %s", m.Name)
		}

		// Rebase succeeded -- update metadata and clear continue state
		if err := meta.Set(m.Name, m.Parent, parentSHA); err != nil {
			return fmt.Errorf("updating metadata for %s: %w", m.Name, err)
		}
		meta.ClearContinueState()

		fmt.Printf("  pushing %s...\n", m.Name)
		if err := git.ForcePush(syncRemote, m.Name); err != nil {
			return fmt.Errorf("pushing %s: %w", m.Name, err)
		}

		pr, _ := gh.PRForBranch(m.Name)
		if pr != nil && pr.BaseRefName != m.Parent {
			fmt.Printf("  updating PR #%d base: %s -> %s\n", pr.Number, pr.BaseRefName, m.Parent)
			if err := gh.EditPRBase(pr.Number, m.Parent); err != nil {
				return fmt.Errorf("updating PR base for %s: %w", m.Name, err)
			}
		}

		commits, _ := git.RevList(m.Parent + ".." + m.Name)
		fmt.Printf("  done (%d commits)\n", len(commits))
		rebased++
	}

	// Step 5: Clean up merged branches
	for _, m := range merged {
		fmt.Printf("\nCleaning up merged branch %s...\n", m.Name)
		if !syncDryRun {
			meta.Delete(m.Name)
			git.Run("branch", "-D", m.Name)
		}
	}

	restoreCheckout(origBranch, syncDryRun)

	if rebased == 0 && len(merged) == 0 {
		fmt.Println("\nEverything is up to date.")
	} else {
		fmt.Printf("\nSync complete. %d branches rebased, %d merged branches cleaned up.\n", rebased, len(merged))
	}
	return nil
}

func ensureCleanTree() error {
	// Only check for staged/unstaged changes, not untracked files.
	// Untracked files are harmless for rebase.
	out, err := git.Run("diff", "--quiet")
	if err != nil {
		return fmt.Errorf("unstaged changes in working tree, commit or stash first")
	}
	out, err = git.Run("diff", "--cached", "--quiet")
	if err != nil {
		return fmt.Errorf("staged changes in index, commit or stash first")
	}
	_ = out
	return nil
}

// recoverFromContinue checks if a previous sync was interrupted by a conflict.
// If the user resolved it (git rebase --continue), we update metadata, push, and proceed.
// If the user aborted (git rebase --abort), we clean up and proceed.
func recoverFromContinue() error {
	cs, err := meta.LoadContinueState()
	if err != nil {
		return err
	}
	if cs == nil {
		return nil
	}

	if meta.RebaseInProgress() {
		return fmt.Errorf("rebase still in progress for %s\nFinish with: git rebase --continue\nOr abort with: git rebase --abort\nThen run: st sync", cs.Branch)
	}

	// Rebase was completed (--continue) or aborted (--abort).
	currentBase, err := git.MergeBase(cs.Branch, cs.Parent)
	if err != nil {
		meta.ClearContinueState()
		return nil
	}

	if currentBase == cs.ParentSHA {
		// Rebase completed successfully -- update metadata and push
		fmt.Printf("Recovering: %s was successfully rebased onto %s\n", cs.Branch, cs.Parent)
		if err := meta.Set(cs.Branch, cs.Parent, cs.ParentSHA); err != nil {
			return err
		}
		fmt.Printf("  pushing %s...\n", cs.Branch)
		if err := git.ForcePush("origin", cs.Branch); err != nil {
			return fmt.Errorf("pushing recovered branch %s: %w", cs.Branch, err)
		}
	} else {
		fmt.Printf("Recovering: rebase of %s was aborted, will retry\n", cs.Branch)
	}

	meta.ClearContinueState()
	return nil
}

func restoreCheckout(branch string, dryRun bool) {
	if dryRun {
		return
	}
	if git.BranchExists(branch) {
		git.Checkout(branch)
	}
}

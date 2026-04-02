package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/liebl/stack/internal/provider"
)

func requireNotTrunk(current, trunk string) error {
	if current == trunk {
		return fmt.Errorf("cannot run this command from %s -- check out a branch in the stack first", trunk)
	}
	return nil
}

type rebaseConflictError struct {
	Branch string
}

func (e *rebaseConflictError) Error() string {
	return fmt.Sprintf("rebase conflict on %s", e.Branch)
}

type rebaseResult struct {
	Rebased int
	Pushed  []string
}

func ensureCleanTree() error {
	if _, err := git.Run("diff", "--quiet"); err != nil {
		return fmt.Errorf("unstaged changes in working tree, commit or stash first")
	}
	if _, err := git.Run("diff", "--cached", "--quiet"); err != nil {
		return fmt.Errorf("staged changes in index, commit or stash first")
	}
	return nil
}

func syncPreamble(remote, trunk string, dryRun bool) (origBranch string, swState *git.SkipWorktreeState, err error) {
	if !dryRun {
		if err := ensureCleanTree(); err != nil {
			return "", nil, err
		}
	}

	origBranch, _ = git.CurrentBranch()

	if !dryRun {
		git.RecoverSkipWorktree()
		swState, err = git.SaveSkipWorktree()
		if err != nil {
			return "", nil, fmt.Errorf("saving skip-worktree state: %w", err)
		}
		if len(swState.Files()) > 0 {
			fmt.Printf("Saved %d skip-worktree files\n", len(swState.Files()))
		}
	}

	fmt.Printf("Pulling %s/%s...\n", remote, trunk)
	if !dryRun {
		if err := git.Checkout(trunk); err != nil {
			return origBranch, swState, fmt.Errorf("checking out %s: %w", trunk, err)
		}
		if err := git.Pull(remote, trunk); err != nil {
			return origBranch, swState, fmt.Errorf("pulling %s: %w", trunk, err)
		}
	}

	return origBranch, swState, nil
}

// rebaseTrackedBranches walks alive branches in topo order and rebases any
// whose base doesn't match their parent's current tip.
func rebaseTrackedBranches(alive []meta.BranchMeta, remote string, dryRun bool, h provider.Host) (*rebaseResult, error) {
	result := &rebaseResult{}
	simulatedTips := make(map[string]string)

	for i, m := range alive {
		parentSHA, err := git.SHA(m.Parent)
		if err != nil {
			return result, fmt.Errorf("resolving parent %s: %w", m.Parent, err)
		}

		if sim, ok := simulatedTips[m.Parent]; ok {
			parentSHA = sim
		}

		if m.Base == parentSHA {
			continue
		}

		// Detect if branch was manually rebased and is already up-to-date.
		// If merge-base(parent, branch) == parentSHA, the branch already
		// sits on the parent's tip and only the stored metadata is stale.
		actualBase, mbErr := git.MergeBase(m.Parent, m.Name)
		if mbErr == nil && actualBase == parentSHA {
			fmt.Printf("  %s: already up-to-date, fixing stale metadata\n", m.Name)
			if !dryRun {
				if err := meta.Set(m.Name, m.Parent, parentSHA); err != nil {
					return result, fmt.Errorf("updating metadata for %s: %w", m.Name, err)
				}
			}
			continue
		}

		fmt.Printf("\nRebasing %s onto %s (base: %s)...\n", m.Name, m.Parent, m.Base[:7])

		if dryRun {
			commits, _ := git.RevList(m.Base + ".." + m.Name)
			fmt.Printf("  would replay %d commits\n", len(commits))
			simulatedTips[m.Name] = "simulated-" + m.Name
			alive[i].Base = parentSHA
			result.Rebased++
			continue
		}

		// Save continue state BEFORE attempting rebase
		meta.SaveContinueState(meta.ContinueState{
			Branch:    m.Name,
			Parent:    m.Parent,
			ParentSHA: parentSHA,
		})

		if err := git.RebaseOnto(m.Parent, m.Base, m.Name); err != nil {
			fmt.Printf("\nConflict rebasing %s. To resolve:\n", m.Name)
			fmt.Printf("  1. Fix conflicts and run: git rebase --continue\n")
			fmt.Printf("  2. Then run: st sync\n")
			fmt.Printf("  Or abort with: git rebase --abort && st sync\n")
			return result, &rebaseConflictError{Branch: m.Name}
		}

		// Push FIRST, then update metadata (Bug 1 fix)
		fmt.Printf("  pushing %s...\n", m.Name)
		if err := git.ForcePush(remote, m.Name); err != nil {
			return result, fmt.Errorf("pushing %s: %w", m.Name, err)
		}

		if err := meta.Set(m.Name, m.Parent, parentSHA); err != nil {
			return result, fmt.Errorf("updating metadata for %s: %w", m.Name, err)
		}
		meta.ClearContinueState()

		// Update PR base if needed (best-effort; provider may not be available)
		if h != nil {
			pr, err := h.PRForBranch(m.Name)
			if err != nil {
				fmt.Printf("  warning: could not fetch PR for %s: %v\n", m.Name, err)
			}
			if pr != nil && pr.Base != m.Parent {
				fmt.Printf("  updating PR #%d base: %s -> %s\n", pr.Number, pr.Base, m.Parent)
				if err := h.EditPRBase(pr.Number, m.Parent); err != nil {
					return result, fmt.Errorf("updating PR base for %s: %w", m.Name, err)
				}
			}
		}

		commits, _ := git.RevList(m.Parent + ".." + m.Name)
		fmt.Printf("  done (%d commits)\n", len(commits))
		result.Rebased++
		result.Pushed = append(result.Pushed, m.Name)
	}

	return result, nil
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
		// Rebase completed successfully -- push FIRST, then update metadata
		fmt.Printf("Recovering: %s was successfully rebased onto %s\n", cs.Branch, cs.Parent)
		fmt.Printf("  pushing %s...\n", cs.Branch)
		if err := git.ForcePush("origin", cs.Branch); err != nil {
			meta.ClearContinueState()
			return fmt.Errorf("pushing recovered branch %s: %w", cs.Branch, err)
		}
		if err := meta.Set(cs.Branch, cs.Parent, cs.ParentSHA); err != nil {
			meta.ClearContinueState()
			return err
		}
	} else {
		fmt.Printf("Recovering: rebase of %s was aborted, will re-attempt during this sync\n", cs.Branch)
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

func confirmAction(prompt string) error {
	fmt.Printf("%s [y/N] ", prompt)
	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	if response != "y" && response != "yes" {
		return fmt.Errorf("aborted")
	}
	return nil
}

func isConflictError(err error) bool {
	var ce *rebaseConflictError
	return errors.As(err, &ce)
}

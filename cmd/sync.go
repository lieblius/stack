package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/liebl/stack/internal/provider"
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

	all, err := meta.AllTracked()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("no tracked branches")
	}

	origBranch, swState, err := syncPreamble(syncRemote, syncTrunk, syncDryRun)
	if err != nil {
		return err
	}

	conflicted := false
	defer func() {
		if !conflicted && swState != nil {
			swState.Restore()
		}
		restoreCheckout(origBranch, syncDryRun)
	}()

	// Identify merged branches and reparent their children
	var merged []meta.BranchMeta
	var alive []meta.BranchMeta
	for _, m := range all {
		pr, err := host.PRForBranch(m.Name)
		if err != nil {
			fmt.Printf("  warning: could not fetch PR for %s: %v\n", m.Name, err)
		}
		if pr != nil && pr.State == provider.PRMerged {
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

	// Rebase alive branches
	result, err := rebaseTrackedBranches(alive, syncRemote, syncDryRun, host)
	if err != nil {
		if isConflictError(err) {
			conflicted = true
		}
		return err
	}

	// Clean up merged branches
	for _, m := range merged {
		fmt.Printf("\nCleaning up merged branch %s...\n", m.Name)
		if !syncDryRun {
			meta.Delete(m.Name)
			git.DeleteLocalBranch(m.Name)
		}
	}

	if result.Rebased == 0 && len(merged) == 0 {
		fmt.Println("\nEverything is up to date.")
	} else {
		fmt.Printf("\nSync complete. %d branches rebased, %d merged branches cleaned up.\n", result.Rebased, len(merged))
	}
	return nil
}

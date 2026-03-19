package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var rebaseCmd = &cobra.Command{
	Use:   "rebase",
	Short: "Rebase stack onto updated trunk",
	Long: `Pull latest trunk and rebase all tracked branches onto the new trunk tip.
Unlike sync, this does not detect or clean up merged branches.`,
	RunE: runRebase,
}

var (
	rebaseTrunk  string
	rebaseRemote string
	rebaseDryRun bool
)

func init() {
	rebaseCmd.Flags().StringVar(&rebaseTrunk, "trunk", "main", "trunk branch name")
	rebaseCmd.Flags().StringVar(&rebaseRemote, "remote", "origin", "remote name")
	rebaseCmd.Flags().BoolVar(&rebaseDryRun, "dry-run", false, "show what would happen without doing it")
}

func runRebase(cmd *cobra.Command, args []string) error {
	_ = requireProvider() // best-effort; rebase works without a provider

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

	origBranch, swState, err := syncPreamble(rebaseRemote, rebaseTrunk, rebaseDryRun)
	if err != nil {
		return err
	}

	conflicted := false
	defer func() {
		if !conflicted && swState != nil {
			swState.Restore()
		}
		restoreCheckout(origBranch, rebaseDryRun)
	}()

	result, err := rebaseTrackedBranches(all, rebaseRemote, rebaseDryRun, host)
	if err != nil {
		if isConflictError(err) {
			conflicted = true
		}
		return err
	}

	if result.Rebased == 0 {
		fmt.Println("\nEverything is up to date.")
	} else {
		fmt.Printf("\nRebase complete. %d branches rebased.\n", result.Rebased)
	}
	return nil
}

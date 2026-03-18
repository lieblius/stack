package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var continueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Resume after resolving a rebase conflict",
	Long: `After resolving a rebase conflict (git rebase --continue),
run this command to push the recovered branch and update metadata.
Then run 'st sync' to continue the cascade.`,
	RunE: runContinue,
}

func runContinue(cmd *cobra.Command, args []string) error {
	cs, err := meta.LoadContinueState()
	if err != nil {
		return err
	}
	if cs == nil {
		return fmt.Errorf("no interrupted operation to continue")
	}

	if meta.RebaseInProgress() {
		return fmt.Errorf("rebase still in progress for %s\nFinish with: git rebase --continue\nOr abort with: git rebase --abort\nThen run: st continue", cs.Branch)
	}

	if err := recoverFromContinue(); err != nil {
		return err
	}

	fmt.Println("Recovery complete. Run 'st sync' to continue the cascade.")
	return nil
}

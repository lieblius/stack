package cmd

import (
	"fmt"
	"strings"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var trackCmd = &cobra.Command{
	Use:   "track [branch1] [branch2] [branch3...]",
	Short: "Track existing branches as a stack",
	Long: `Adopt existing branches into a tracked stack. Branches are listed
bottom to top. The first branch's parent is the trunk (default: main).

To specify an explicit fork-point (for partially-rebased stacks):
  st track feature-1 feature-2:abc1234 feature-3

Example:
  st track feature-1 feature-2 feature-3`,
	Args: cobra.MinimumNArgs(1),
	RunE: runTrack,
}

var (
	trackTrunk string
	trackForce bool
)

func init() {
	trackCmd.Flags().StringVar(&trackTrunk, "trunk", "main", "trunk branch name")
	trackCmd.Flags().BoolVar(&trackForce, "force", false, "overwrite existing tracking metadata")
}

// parseBranchArg parses "branch" or "branch:sha" syntax.
func parseBranchArg(arg string) (branch, explicitBase string) {
	if i := strings.LastIndex(arg, ":"); i > 0 {
		return arg[:i], arg[i+1:]
	}
	return arg, ""
}

func runTrack(cmd *cobra.Command, args []string) error {
	// Parse and validate all branches first
	type entry struct {
		branch, explicitBase string
	}
	var entries []entry
	for _, arg := range args {
		branch, base := parseBranchArg(arg)
		if !git.BranchExists(branch) {
			return fmt.Errorf("branch %q does not exist locally", branch)
		}
		if !trackForce {
			existing, err := meta.Get(branch)
			if err != nil {
				return err
			}
			if existing != nil {
				return fmt.Errorf("branch %s already tracked (parent: %s). Use --force to overwrite", branch, existing.Parent)
			}
		}
		entries = append(entries, entry{branch, base})
	}

	parent := trackTrunk
	for _, e := range entries {
		var base string
		if e.explicitBase != "" {
			// Resolve explicit base to full SHA
			full, err := git.SHA(e.explicitBase)
			if err != nil {
				return fmt.Errorf("invalid base SHA %q for %s: %w", e.explicitBase, e.branch, err)
			}
			base = full
		} else {
			var err error
			base, err = git.MergeBase(e.branch, parent)
			if err != nil {
				return fmt.Errorf("computing merge-base for %s and %s: %w", e.branch, parent, err)
			}

			// Warn if merge-base doesn't match parent tip (parent has new commits)
			parentSHA, _ := git.SHA(parent)
			if parentSHA != "" && base != parentSHA {
				fmt.Printf("  note: %s is behind %s (run 'st rebase' to update)\n",
					e.branch, parent)
			}
		}

		if err := meta.Set(e.branch, parent, base); err != nil {
			return err
		}

		shortBase, _ := git.ShortSHA(base)
		fmt.Printf("  %s -> %s (base: %s)\n", e.branch, parent, shortBase)
		parent = e.branch
	}

	fmt.Printf("\nTracked %d branches.\n", len(entries))
	return nil
}

package cmd

import (
	"fmt"
	"strconv"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:     "up [n]",
	Aliases: []string{"u"},
	Short:   "Navigate to child branch",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runUp,
}

var downCmd = &cobra.Command{
	Use:     "down [n]",
	Aliases: []string{"d"},
	Short:   "Navigate to parent branch",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runDown,
}

var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Navigate to the top of the stack",
	RunE:  runTop,
}

var bottomCmd = &cobra.Command{
	Use:   "bottom",
	Short: "Navigate to the bottom of the stack",
	RunE:  runBottom,
}

func parseSteps(args []string) (int, error) {
	if len(args) == 0 {
		return 1, nil
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid step count: %s", args[0])
	}
	return n, nil
}

func pickChild(branch string, children []string) (string, error) {
	if len(children) == 1 {
		return children[0], nil
	}
	fmt.Printf("Multiple children of %s:\n", branch)
	for i, child := range children {
		fmt.Printf("  %d. %s\n", i+1, child)
	}
	fmt.Printf("Choose [1-%d]: ", len(children))
	var choice int
	fmt.Scanln(&choice)
	if choice < 1 || choice > len(children) {
		return "", fmt.Errorf("invalid choice")
	}
	return children[choice-1], nil
}

func runUp(cmd *cobra.Command, args []string) error {
	steps, err := parseSteps(args)
	if err != nil {
		return err
	}

	current, err := git.CurrentBranch()
	if err != nil {
		return err
	}

	all, err := meta.AllTracked()
	if err != nil {
		return err
	}

	branch := current
	for i := 0; i < steps; i++ {
		children := meta.Children(branch, all)
		if len(children) == 0 {
			if i == 0 {
				return fmt.Errorf("no child branches above %s", branch)
			}
			break
		}
		next, err := pickChild(branch, children)
		if err != nil {
			return err
		}
		branch = next
	}

	if branch == current {
		return nil
	}
	fmt.Println(branch)
	return git.Checkout(branch)
}

func runDown(cmd *cobra.Command, args []string) error {
	steps, err := parseSteps(args)
	if err != nil {
		return err
	}

	current, err := git.CurrentBranch()
	if err != nil {
		return err
	}

	branch := current
	for i := 0; i < steps; i++ {
		m, err := meta.Get(branch)
		if err != nil {
			return err
		}
		if m == nil {
			if i == 0 {
				return fmt.Errorf("%s is not tracked", branch)
			}
			break
		}
		branch = m.Parent
	}

	if branch == current {
		return nil
	}
	fmt.Println(branch)
	return git.Checkout(branch)
}

func runTop(cmd *cobra.Command, args []string) error {
	current, err := git.CurrentBranch()
	if err != nil {
		return err
	}

	all, err := meta.AllTracked()
	if err != nil {
		return err
	}

	branch := current
	for {
		children := meta.Children(branch, all)
		if len(children) == 0 {
			break
		}
		next, err := pickChild(branch, children)
		if err != nil {
			return err
		}
		branch = next
	}

	if branch == current {
		fmt.Println("Already at the top.")
		return nil
	}
	fmt.Println(branch)
	return git.Checkout(branch)
}

func runBottom(cmd *cobra.Command, args []string) error {
	current, err := git.CurrentBranch()
	if err != nil {
		return err
	}

	all, err := meta.AllTracked()
	if err != nil {
		return err
	}

	// If current is tracked, walk up to the bottom (first branch whose parent is untracked)
	m, _ := meta.Get(current)
	if m != nil {
		branch := current
		for {
			bm, _ := meta.Get(branch)
			if bm == nil {
				break
			}
			pm, _ := meta.Get(bm.Parent)
			if pm == nil {
				// bm.Parent is untracked (trunk), so branch is the bottom
				break
			}
			branch = bm.Parent
		}
		if branch == current {
			fmt.Println("Already at the bottom.")
			return nil
		}
		fmt.Println(branch)
		return git.Checkout(branch)
	}

	// Current branch is not tracked (e.g., trunk). Go to first tracked child.
	children := meta.Children(current, all)
	if len(children) == 0 {
		return fmt.Errorf("no tracked branches below %s", current)
	}
	next, err := pickChild(current, children)
	if err != nil {
		return err
	}
	fmt.Println(next)
	return git.Checkout(next)
}

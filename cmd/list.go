package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/liebl/stack/internal/gh"
	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "Show the current stack",
	RunE:    runList,
}

var (
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	branchStyle = lipgloss.NewStyle().Bold(true)
	mergedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	openStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	currentMark = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render("*")
)

func runList(cmd *cobra.Command, args []string) error {
	all, err := meta.AllTracked()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Println("No tracked branches. Use 'st track' to add branches.")
		return nil
	}

	current, _ := git.CurrentBranch()

	// Find trunk(s) -- parents that aren't tracked branches
	trackedSet := make(map[string]bool)
	for _, m := range all {
		trackedSet[m.Name] = true
	}

	trunks := make(map[string]bool)
	for _, m := range all {
		if !trackedSet[m.Parent] {
			trunks[m.Parent] = true
		}
	}

	for trunk := range trunks {
		fmt.Println(dimStyle.Render(trunk))
		printChildren(trunk, all, current, 1)
	}

	return nil
}

func printChildren(parent string, all []meta.BranchMeta, current string, depth int) {
	children := meta.Children(parent, all)
	for _, child := range children {
		prefix := strings.Repeat("  ", depth)
		marker := " "
		if child == current {
			marker = currentMark
		}

		prInfo := ""
		if pr, err := gh.PRForBranch(child); err == nil && pr != nil {
			stateStr := formatPRState(pr.State)
			prInfo = fmt.Sprintf("  PR #%d %s", pr.Number, stateStr)
		}

		commits, _ := git.RevList(parent + ".." + child)
		commitCount := ""
		if len(commits) > 0 {
			commitCount = dimStyle.Render(fmt.Sprintf(" (%d commits)", len(commits)))
		}

		fmt.Printf("%s%s %s%s%s\n", prefix, marker, branchStyle.Render(child), prInfo, commitCount)
		printChildren(child, all, current, depth+1)
	}
}

func formatPRState(state string) string {
	switch state {
	case "MERGED":
		return mergedStyle.Render("[MERGED]")
	case "OPEN":
		return openStyle.Render("[OPEN]")
	case "CLOSED":
		return dimStyle.Render("[CLOSED]")
	default:
		return dimStyle.Render("[" + state + "]")
	}
}

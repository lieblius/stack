package meta

import (
	"fmt"
	"strings"

	"github.com/liebl/stack/internal/git"
)

const configPrefix = "stack.branch."
const trunkKey = "stack.trunk"

// Trunk returns the configured trunk branch name. If not set, auto-detects
// from existing branches (main, then master) and persists the result.
func Trunk() string {
	name, err := git.Run("config", "--local", trunkKey)
	if err == nil && name != "" {
		return name
	}
	// Auto-detect: prefer "main", fall back to "master", default to "main"
	detected := "main"
	if !git.BranchExists("main") && git.BranchExists("master") {
		detected = "master"
	}
	// Persist so we only detect once
	SetTrunk(detected)
	return detected
}

// SetTrunk persists the trunk branch name in git config.
func SetTrunk(name string) error {
	_, err := git.Run("config", "--local", trunkKey, name)
	return err
}

type BranchMeta struct {
	Name   string
	Parent string // parent branch name
	Base   string // fork-point SHA (parentBranchRevision)
}

func Get(branch string) (*BranchMeta, error) {
	parent, err := git.Run("config", "--local", configPrefix+branch+".parent")
	if err != nil {
		return nil, nil // no metadata = untracked
	}
	base, _ := git.Run("config", "--local", configPrefix+branch+".base")
	return &BranchMeta{
		Name:   branch,
		Parent: parent,
		Base:   base,
	}, nil
}

func Set(branch, parent, base string) error {
	if branch == parent {
		return fmt.Errorf("cannot set branch %s as its own parent", branch)
	}
	if _, err := git.Run("config", "--local", configPrefix+branch+".parent", parent); err != nil {
		return fmt.Errorf("setting parent for %s: %w", branch, err)
	}
	if _, err := git.Run("config", "--local", configPrefix+branch+".base", base); err != nil {
		return fmt.Errorf("setting base for %s: %w", branch, err)
	}
	return nil
}

func Delete(branch string) error {
	git.Run("config", "--local", "--remove-section", configPrefix+branch)
	return nil
}

func SetParent(branch, parent string) error {
	if branch == parent {
		return fmt.Errorf("cannot set branch %s as its own parent", branch)
	}
	_, err := git.Run("config", "--local", configPrefix+branch+".parent", parent)
	return err
}

func SetBase(branch, base string) error {
	_, err := git.Run("config", "--local", configPrefix+branch+".base", base)
	return err
}

// AllTracked returns all branches that have stack metadata, ordered by walking
// the parent chain from trunk to tips.
func AllTracked() ([]BranchMeta, error) {
	out, err := git.Run("config", "--local", "--get-regexp", `^stack\.branch\..*\.parent$`)
	if err != nil {
		return nil, nil // no metadata at all
	}

	// Parse raw config lines: "stack.branch.foo.parent main"
	byName := make(map[string]*BranchMeta)
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		parent := parts[1]

		// Extract branch name from key: stack.branch.<name>.parent
		name := strings.TrimPrefix(key, configPrefix)
		name = strings.TrimSuffix(name, ".parent")

		base, _ := git.Run("config", "--local", configPrefix+name+".base")
		byName[name] = &BranchMeta{
			Name:   name,
			Parent: parent,
			Base:   base,
		}
	}

	return topoSort(byName)
}

// topoSort orders branches from trunk toward tips. Returns error on cycle.
func topoSort(byName map[string]*BranchMeta) ([]BranchMeta, error) {
	var result []BranchMeta

	const (
		unvisited  = 0
		inProgress = 1
		done       = 2
	)
	state := make(map[string]int)

	var visit func(name string) error
	visit = func(name string) error {
		switch state[name] {
		case done:
			return nil
		case inProgress:
			return fmt.Errorf("cycle detected involving branch %s", name)
		}
		state[name] = inProgress
		m, ok := byName[name]
		if !ok {
			state[name] = done
			return nil
		}
		if _, hasParent := byName[m.Parent]; hasParent {
			if err := visit(m.Parent); err != nil {
				return err
			}
		}
		state[name] = done
		result = append(result, *m)
		return nil
	}

	for name := range byName {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Children returns the names of branches whose parent is the given branch.
func Children(parent string, all []BranchMeta) []string {
	var children []string
	for _, m := range all {
		if m.Parent == parent {
			children = append(children, m.Name)
		}
	}
	return children
}

// StackFromBranch returns the ordered stack starting from trunk, passing
// through the given branch. Walks up to find trunk, then down to find tips.
func StackFromBranch(branch string) ([]BranchMeta, error) {
	all, err := AllTracked()
	if err != nil {
		return nil, err
	}

	// Find all branches in the same connected stack
	inStack := make(map[string]bool)

	// Walk up from branch to trunk with cycle detection
	cur := branch
	seen := make(map[string]bool)
	for {
		if seen[cur] {
			break
		}
		seen[cur] = true
		inStack[cur] = true
		m := findMeta(cur, all)
		if m == nil {
			break
		}
		cur = m.Parent
	}

	// Walk down from branch to find all descendants
	var walkDown func(name string)
	walkDown = func(name string) {
		for _, child := range Children(name, all) {
			inStack[child] = true
			walkDown(child)
		}
	}
	walkDown(branch)

	var result []BranchMeta
	for _, m := range all {
		if inStack[m.Name] {
			result = append(result, m)
		}
	}
	return result, nil
}

// PruneStale removes tracked branches that no longer exist locally,
// reparenting their children to the pruned branch's parent.
func PruneStale() ([]string, error) {
	all, err := AllTracked()
	if err != nil {
		return nil, err
	}

	var pruned []string
	for _, m := range all {
		if !git.BranchExists(m.Name) {
			pruned = append(pruned, m.Name)
		}
	}

	if len(pruned) == 0 {
		return nil, nil
	}

	prunedSet := make(map[string]bool)
	for _, name := range pruned {
		prunedSet[name] = true
	}

	// Reparent children of pruned branches
	for _, name := range pruned {
		m, err := Get(name)
		if err != nil || m == nil {
			continue
		}
		for _, other := range all {
			if other.Parent == name && !prunedSet[other.Name] {
				SetParent(other.Name, m.Parent)
			}
		}
		Delete(name)
	}

	return pruned, nil
}

func findMeta(name string, all []BranchMeta) *BranchMeta {
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

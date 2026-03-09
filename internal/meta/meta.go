package meta

import (
	"fmt"
	"strings"

	"github.com/liebl/stack/internal/git"
)

const configPrefix = "stack.branch."

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

	return topoSort(byName), nil
}

// topoSort orders branches from trunk toward tips.
func topoSort(byName map[string]*BranchMeta) []BranchMeta {
	var result []BranchMeta
	visited := make(map[string]bool)

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		m, ok := byName[name]
		if !ok {
			return
		}
		// Visit parent first (if it's tracked)
		if _, hasParent := byName[m.Parent]; hasParent {
			visit(m.Parent)
		}
		result = append(result, *m)
	}

	for name := range byName {
		visit(name)
	}
	return result
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

	// Walk up from branch to trunk
	cur := branch
	for {
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

func findMeta(name string, all []BranchMeta) *BranchMeta {
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

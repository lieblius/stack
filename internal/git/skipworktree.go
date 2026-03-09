package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SkipWorktreeState struct {
	files    []string
	tmpDir   string
	repoDir  string
	restored bool
}

// SaveSkipWorktree finds all skip-worktree files, backs them up, clears
// the skip-worktree bit, and restores clean index versions so the working
// tree is clean for rebase operations.
func SaveSkipWorktree() (*SkipWorktreeState, error) {
	out, err := Run("ls-files", "-v")
	if err != nil {
		return nil, err
	}

	var files []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "S ") {
			files = append(files, strings.TrimPrefix(line, "S "))
		}
	}

	if len(files) == 0 {
		return &SkipWorktreeState{}, nil
	}

	repoDir, err := Run("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "st-skip-worktree-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	state := &SkipWorktreeState{
		files:   files,
		tmpDir:  tmpDir,
		repoDir: repoDir,
	}

	for _, f := range files {
		src := filepath.Join(repoDir, f)
		dst := filepath.Join(tmpDir, f)

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			state.Restore()
			return nil, fmt.Errorf("creating backup dir for %s: %w", f, err)
		}

		data, err := os.ReadFile(src)
		if err != nil {
			// File might not exist on disk, that's ok
			continue
		}

		if err := os.WriteFile(dst, data, 0644); err != nil {
			state.Restore()
			return nil, fmt.Errorf("backing up %s: %w", f, err)
		}

		// Clear skip-worktree bit
		if _, err := Run("update-index", "--no-skip-worktree", f); err != nil {
			state.Restore()
			return nil, fmt.Errorf("clearing skip-worktree for %s: %w", f, err)
		}

		// Restore the clean index version so working tree isn't dirty
		if _, err := Run("checkout", "--", f); err != nil {
			// Not fatal -- file might not be in current branch
			continue
		}
	}

	return state, nil
}

func (s *SkipWorktreeState) Files() []string {
	return s.files
}

// Restore puts skip-worktree files back and re-sets the bit.
// Safe to call multiple times (idempotent).
func (s *SkipWorktreeState) Restore() {
	if len(s.files) == 0 || s.restored {
		return
	}
	s.restored = true

	for _, f := range s.files {
		src := filepath.Join(s.tmpDir, f)
		dst := filepath.Join(s.repoDir, f)

		if data, err := os.ReadFile(src); err == nil {
			os.MkdirAll(filepath.Dir(dst), 0755)
			os.WriteFile(dst, data, 0644)
		}

		// Re-set skip-worktree bit (best effort)
		Run("update-index", "--skip-worktree", f)
	}

	os.RemoveAll(s.tmpDir)
}

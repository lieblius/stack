package meta

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/liebl/stack/internal/git"
)

type ContinueState struct {
	Branch    string `json:"branch"`     // branch that was being rebased
	Parent    string `json:"parent"`     // intended parent
	ParentSHA string `json:"parent_sha"` // intended new base (parent's SHA at rebase time)
}

func continueFilePath() (string, error) {
	gitDir, err := git.Run("rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, "st-continue.json"), nil
}

func SaveContinueState(state ContinueState) error {
	path, err := continueFilePath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func LoadContinueState() (*ContinueState, error) {
	path, err := continueFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state ContinueState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func ClearContinueState() error {
	path, err := continueFilePath()
	if err != nil {
		return err
	}
	os.Remove(path)
	return nil
}

func RebaseInProgress() bool {
	gitDir, err := git.Run("rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	// git creates rebase-merge/ or rebase-apply/ during rebase
	for _, dir := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(gitDir, dir)); err == nil {
			return true
		}
	}
	return false
}

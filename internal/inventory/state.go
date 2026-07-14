package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ToolState struct {
	Version string `json:"version"`
	Manager string `json:"manager"`
}

type State struct {
	Tools map[string]ToolState `json:"tools"`
}

func LoadState(path string) (State, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{Tools: map[string]ToolState{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	if s.Tools == nil {
		s.Tools = map[string]ToolState{}
	}
	return s, nil
}

func (s State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

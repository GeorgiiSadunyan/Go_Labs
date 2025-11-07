package storage

import (
	"encoding/json"
	"os"
)

type FileStorage struct {
	filename string
}

type State struct {
	Variables map[string]float64 `json:"variables"`
	StringVariables map[string]string `json:"string_variables"` // ← новое поле
	History   []string           `json:"history"`
}

func NewFileStorage(filename string) *FileStorage {
	return &FileStorage{filename: filename}
}

func (s *FileStorage) Load() (map[string]float64, map[string]string, []string, error) {
	file, err := os.Open(s.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]float64), make(map[string]string), []string{}, nil
		}
		return nil, nil, nil, err
	}
	defer file.Close()

	var state State
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return nil, nil, nil, err
	}

	if state.Variables == nil {
		state.Variables = make(map[string]float64)
	}
	if state.StringVariables == nil {
		state.StringVariables = make(map[string]string)
	}
	if state.History == nil {
		state.History = []string{}
	}

	return state.Variables, state.StringVariables, state.History, nil
}

func (s *FileStorage) Save(vars map[string]float64, strVars map[string]string, history []string) error {
	state := State{
		Variables: vars,
		StringVariables: strVars,
		History:   history,
	}

	file, err := os.Create(s.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(state)
}

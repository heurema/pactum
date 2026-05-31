package ledger

import (
	"encoding/json"
	"os"
	"time"
)

type Event struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	RunID     string    `json:"run_id,omitempty"`
	RepoRoot  string    `json:"repo_root,omitempty"`
}

func Append(path string, event Event) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = file.Write(encoded)
	return err
}

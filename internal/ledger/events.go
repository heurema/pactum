package ledger

import (
	"encoding/json"
	"time"

	"github.com/heurema/pactum/internal/store"
)

type Event struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	RunID     string    `json:"run_id,omitempty"`
	RepoRoot  string    `json:"repo_root,omitempty"`
}

func Append(store store.Store, path string, event Event) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return store.AppendBytes(path, encoded)
}

package monitor

import (
	"encoding/json"
	"fmt"
	"os"
)

type Metadata struct {
	ClusterID string
	ProcessID uint64
}

func (m *Metadata) Save(file string) error {
	contents, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling: %w", err)
	}
	return os.WriteFile(file, contents, 0600)
}

func (m *Metadata) Load(file string) error {
	contents, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	if err := json.Unmarshal(contents, m); err != nil {
		return fmt.Errorf("parsing json: %w", err)
	}
	return nil
}

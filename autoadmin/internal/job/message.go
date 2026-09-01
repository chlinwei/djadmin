package job

import (
	"encoding/json"
	"time"
)

const SchemaVersion = 1

type Message struct {
	SchemaVersion int             `json:"schema_version"`
	ExecutionID   string          `json:"execution_id"`
	Kind          string          `json:"kind"`
	ResourceID    int64           `json:"resource_id"`
	TriggeredAt   time.Time       `json:"triggered_at"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

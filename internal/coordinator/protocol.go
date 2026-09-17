package coordinator

import "encoding/json"

const protocolVersion = 2

type Request struct {
	Token      string         `json:"token"`
	Operation  string         `json:"operation"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	Freshness  string         `json:"freshness,omitempty"`
	SnapshotID string         `json:"snapshot_id,omitempty"`
}

type Metadata struct {
	SnapshotID       string `json:"snapshot_id"`
	RuleRevision     int64  `json:"rule_revision"`
	Freshness        string `json:"freshness"`
	ActiveBaseline   string `json:"active_baseline"`
	RefreshTarget    string `json:"refresh_target,omitempty"`
	RefreshTaskCount int    `json:"refresh_task_count"`
}

type Response struct {
	Result json.RawMessage `json:"result,omitempty"`
	Meta   Metadata        `json:"meta"`
	Error  string          `json:"error,omitempty"`
}

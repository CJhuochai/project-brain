package workspace

type BaselineState string

const (
	BaselineKnown   BaselineState = "baseline_known"
	BaselineUnknown BaselineState = "baseline_unknown"
)

type Repository struct {
	Path           string        `json:"path"`
	Name           string        `json:"name"`
	RemoteURL      string        `json:"remote_url,omitempty"`
	BaselineBranch string        `json:"baseline_branch,omitempty"`
	BaselineCommit string        `json:"baseline_commit,omitempty"`
	BaselineState  BaselineState `json:"baseline_state"`
}

package query

// Coverage describes this query's coverage of the indexed static evidence;
// Complete never guarantees that all runtime behavior has been modeled.
type Coverage struct {
	Complete    bool     `json:"complete"`
	Truncated   bool     `json:"truncated"`
	Reasons     []string `json:"reasons,omitempty"`
	Unresolved  int      `json:"unresolved"`
	Diagnostics int      `json:"diagnostics"`
	Limit       int      `json:"limit,omitempty"`
	Returned    int      `json:"returned"`
	Matched     int      `json:"matched,omitempty"`
}

func (c *Coverage) Gap(reason string, truncated bool) {
	c.Complete = false
	c.Truncated = c.Truncated || truncated
	for _, existing := range c.Reasons {
		if existing == reason {
			return
		}
	}
	c.Reasons = append(c.Reasons, reason)
}

func (c *Coverage) Merge(other Coverage) {
	if !other.Complete {
		c.Complete = false
	}
	c.Truncated = c.Truncated || other.Truncated
	c.Unresolved += other.Unresolved
	c.Diagnostics += other.Diagnostics
	for _, reason := range other.Reasons {
		c.Gap(reason, other.Truncated)
	}
}

package extract

type Confidence string

const (
	Certain    Confidence = "certain"
	Probable   Confidence = "probable"
	Unresolved Confidence = "unresolved"
)

type Symbol struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Line int    `json:"line"`
}

type Edge struct {
	Source     string     `json:"source"`
	Target     string     `json:"target"`
	Kind       string     `json:"kind"`
	Line       int        `json:"line"`
	Confidence Confidence `json:"confidence"`
}

type Result struct {
	Symbols []Symbol `json:"symbols"`
	Edges   []Edge   `json:"edges"`
}

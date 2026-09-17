package extract

type Confidence string

const (
	Certain    Confidence = "certain"
	Probable   Confidence = "probable"
	Unresolved Confidence = "unresolved"
)

type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Line      int    `json:"line"`
	Signature string `json:"signature,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type Edge struct {
	Source          string     `json:"source"`
	Target          string     `json:"target"`
	Kind            string     `json:"kind"`
	Line            int        `json:"line"`
	Confidence      Confidence `json:"confidence"`
	SourceSignature string     `json:"source_signature,omitempty"`
	TargetArity     *int       `json:"target_arity,omitempty"`
}

type Result struct {
	Symbols     []Symbol     `json:"symbols"`
	Edges       []Edge       `json:"edges"`
	Contracts   []Contract   `json:"contracts"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Contract is a statically observed service boundary. Empty broker means it
// could not be identified from the source and must not be matched as known.
type Contract struct {
	Kind       string     `json:"kind"`
	Role       string     `json:"role"`
	Service    string     `json:"service,omitempty"`
	Key        string     `json:"key"`
	Symbol     string     `json:"symbol"`
	Signature  string     `json:"signature,omitempty"`
	Line       int        `json:"line"`
	Confidence Confidence `json:"confidence"`
	Reason     string     `json:"reason"`
	Broker     string     `json:"broker,omitempty"`
}

type Diagnostic struct {
	Message    string     `json:"message"`
	Line       int        `json:"line"`
	Confidence Confidence `json:"confidence"`
}

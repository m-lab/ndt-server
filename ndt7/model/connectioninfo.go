package model

// ConnectionInfo contains connection info. This structure is described
// in the ndt7 specification.
type ConnectionInfo struct {
	Client string
	Server string
	UUID   string `json:",omitempty"`
}

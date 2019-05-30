package model

// ConnectionInfo contains connection info.
type ConnectionInfo struct {
	// Client is the client endpoint
	Client string `json:"client"`

	// Server is the server endpoint
	Server string `json:"server"`

	// UUID is the internal unique identifier of this experiment.
	UUID string `json:"uuid"`
}

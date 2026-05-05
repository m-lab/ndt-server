package handler

// IntegrationClaims carries m-lab/locate integration-specific JWT claims that
// are attached to NDT7 result ClientMetadata when present in a verified
// access_token.
type IntegrationClaims struct {
	IntegrationID string `json:"int_id,omitempty"`
	KeyID         string `json:"key_id,omitempty"`
}

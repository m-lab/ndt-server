package metadata

// NameValue is a BigQuery-compatible type for ClientMetadata/ServerMetadata "name"/"value" pairs.
type NameValue struct {
	Name  string
	Value string
}

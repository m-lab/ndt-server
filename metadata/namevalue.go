package metadata

// NameValue is a BigQuery-compatible type for ClientMetadata "name"/"value" pairs.
type NameValue struct {
	Name  string
	Value string
}

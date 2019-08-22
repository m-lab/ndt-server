// Package results contains server results
package results

import (
	"net/url"
	"regexp"

	meta "github.com/m-lab/ndt-server/metadata"
)

// metadata contains ndt7 metadata.
type metadata []meta.NameValue

// serverKeyRe is a regexp that matches any server related key.
var serverKeyRe = regexp.MustCompile("^server_")

// initMetadata initializes |*meta| from |values| provided from the original
// request query string.
func initMetadata(m *metadata, values url.Values) {
	for name, values := range values {
		if matches := serverKeyRe.MatchString(name); matches {
			continue // We MUST skip variables reserved to the server
		}
		*m = append(*m, meta.NameValue{Name: name, Value: values[0]})
	}
}

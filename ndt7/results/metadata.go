// Package results contains server results
package results

import (
	"net/url"
	"regexp"
)

// metadata contains ndt7 metadata.
type metadata map[string]string

// serverKeyRe is a regexp that matches any server related key.
var serverKeyRe = regexp.MustCompile("^server_")

// initMetadata initializes |*meta| from |values| provided from the original
// request query string.
func initMetadata(m *metadata, values url.Values) {
	for k, v := range values {
		if matches := serverKeyRe.MatchString(k); matches {
			continue // We MUST skip variables reserved to the server
		}
		(*m)[k] = v[0]
	}
}

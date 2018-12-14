// Package results contains server results
package results

import (
	"net/url"
	"regexp"

	"github.com/m-lab/ndt-cloud/version"
)

// metadata contains ndt7 metadata.
type metadata map[string]string

// serverKeyRe is a regexp that matches any server related key.
var serverKeyRe = regexp.MustCompile("^server_")

// initMetadata initializes |*meta| from |localEpnt|, |remoteEpnt|, the |values|
// provided using the query string, and the |subtest| name. Returns an error
// on failure.
func initMetadata(m *metadata, localEpnt, remoteEpnt string, values url.Values, subtest string) {
	for k, v := range values {
		if matches := serverKeyRe.MatchString(k); matches {
			continue  // We MUST skip variables reserved to the server
		}
		if len(v) != 1 {
			continue  // We SHOULD ignore duplicate keys
		}
		(*m)[k] = v[0]
	}
	(*m)["server_name"] = "ndt-cloud"
	(*m)["server_version"] = version.Version
	(*m)["server_local_endpoint"] = localEpnt
	(*m)["server_remote_endpoint"] = remoteEpnt
	(*m)["server_subtest"] = subtest
}

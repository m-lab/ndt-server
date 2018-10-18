package ndt7

import (
	"errors"
	"net/url"
	"regexp"

	"github.com/m-lab/ndt-cloud/version"
)

// errInvalidMetadata indicates that the query string keys and/or values
// do not match the regular expression specified in the ndt7 spec.
var errInvalidMetadata = errors.New("invalid query string")

// metadata contains ndt7 metadata.
type metadata map[string]string

// initMetadata initializes |*meta| from |localEpnt|, |remoteEpnt|, the |values|
// provided using the query string, and the |subtest| name. Returns an error
// on failure.
func initMetadata(m *metadata, localEpnt, remoteEpnt string, values url.Values, subtest string) error {
	for k, v := range values {
		matches, err := regexp.MatchString("^server_", k)
		if err != nil {
			return err
		}
		if matches {
			continue  // We MUST skip variables reserved to the server
		}
		if len(v) != 1 {
			return errInvalidMetadata  // We MUST fail if there are duplicate keys
		}
		(*m)[k] = v[0]
	}
	(*m)["server_name"] = "ndt-cloud"
	(*m)["server_version"] = version.Version
	(*m)["server_local_endpoint"] = localEpnt
	(*m)["server_remote_endpoint"] = remoteEpnt
	(*m)["server_subtest"] = subtest
	return nil
}

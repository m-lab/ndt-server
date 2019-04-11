// Package uuidx contains a portable wrapper around github.com/m-lab/uuid.
package uuidx

import (
	"os"
)

// FromFile returns a string that is a globally unique identifier for the socket
// represented by the os.File pointer.
//
// On Linux we use github.com/m-lab/uuid. On other platforms we use less
// optimal and supported solutions.
func FromFile(file *os.File) (string, error) {
	return realFromFile(file)
}

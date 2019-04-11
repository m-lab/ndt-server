// +build !linux

package uuidx

import (
	"os"

	"github.com/satori/go.uuid"
)

func realFromFile(file *os.File) (string, error) {
	UUID, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return UUID.String(), nil
}

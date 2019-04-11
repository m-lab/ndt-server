package uuidx

import (
	"os"

	"github.com/m-lab/uuid"
)

func realFromFile(file *os.File) (string, error) {
	return uuid.FromFile(file)
}

package tasks

import (
	"embed"
	"io/fs"
)

//go:embed all:data
var dataFS embed.FS

func Embedded() (fs.FS, error) {
	return fs.Sub(dataFS, "data")
}

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/flexer2006/tco/internal/bootstrap"
)

func main() {
	os.Exit(run(bootstrap.Serve, os.Stderr))
}

func run(serve func() error, stderr io.Writer) int {
	err := serve()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 1
	}

	return 0
}

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/flexer2006/tco/internal/bootstrap"
)

var (
	collectorServe            = bootstrap.Serve
	collectorStderr io.Writer = os.Stderr
	collectorExit             = os.Exit
)

func main() {
	if err := collectorServe(); err != nil {
		_, _ = fmt.Fprintln(collectorStderr, err)
		collectorExit(1)
	}
}

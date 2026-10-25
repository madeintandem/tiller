// Command tiller is the single-binary entrypoint for the Tiller CLI.
package main

import (
	"os"

	"github.com/madeintandem/tiller/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}

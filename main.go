package main

import (
	"os"

	"github.com/rarimo/evm-identity-saver-svc/internal/cli"
)

func main() {
	if !cli.Run(os.Args) {
		os.Exit(1)
	}
}

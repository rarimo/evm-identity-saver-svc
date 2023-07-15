package main

import (
	"os"

	"gitlab.com/rarimo/polygonid/evm-identity-saver-svc/internal/cli"
)

func main() {
	if !cli.Run(os.Args) {
		os.Exit(1)
	}
}

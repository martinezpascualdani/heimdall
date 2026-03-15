package main

import (
	"os"

	"github.com/martinezpascualdani/heimdall/cmd/heimdallctl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

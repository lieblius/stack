package main

import (
	"os"

	"github.com/liebl/stack/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

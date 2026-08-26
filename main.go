package main

import (
	"os"

	"github.com/radjathaher/noise-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}

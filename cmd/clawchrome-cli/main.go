package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gvkhna/clawchrome-cli/internal/bridge"
	"github.com/gvkhna/clawchrome-cli/internal/cli"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__bridge" {
		if err := bridge.Run(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	exitCode := cli.Main(os.Args[1:], os.Stdout, os.Stderr)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

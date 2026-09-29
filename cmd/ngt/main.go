// Command ngt is a personal toolbox of developer utilities.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := cli.NewRootCmd(version).ExecuteContext(ctx)
	stop()
	if err != nil {
		os.Exit(1)
	}
}

package main

import (
	"context"
	"github.com/CavnHan/multichain-sync-account/common/opio"
	"os"

	"github.com/ethereum/go-ethereum/log"
	"github.com/CavnHan/multichain-sync-account/cmd/multichain_sync"
)

var (
	GitCommit = ""
	GitData   = ""
)

func main() {
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))
	app := multichain_sync.NewCli(GitCommit, GitData)
	ctx := opio.WithInterruptBlocker(context.Background())
	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Error("Application failed")
		os.Exit(1)
	}
}

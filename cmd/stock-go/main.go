package main

import (
	"context"
	"errors"
	"github.com/Gavin-Gong/stock-go/mcp"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gavin-Gong/stock-go/internal/cli"
)

var version = "dev"

func main() {
	// A closed downstream pipe is a normal CLI termination, including writes to fd 1.
	signal.Ignore(syscall.SIGPIPE)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], cli.Config{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Version: version, MCP: func(ctx context.Context, in io.Reader, out, log io.Writer) error {
		err := mcp.Start(ctx, in, out, log, mcp.Options{Version: version})
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}})
	cancel()
	os.Exit(code)
}

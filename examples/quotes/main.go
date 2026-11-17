// Run with: go run ./examples/quotes [cn|hk|us|fund] [codes...]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	stockgo "github.com/Gavin-Gong/stock-go"
)

func main() {
	args := os.Args[1:]
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/quotes [cn|hk|us|fund] CODE...")
		os.Exit(2)
	}
	sdk, err := stockgo.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var result any
	switch args[0] {
	case "cn":
		result, err = sdk.Quotes.CN(ctx, args[1:])
	case "hk":
		result, err = sdk.Quotes.HK(ctx, args[1:])
	case "us":
		result, err = sdk.Quotes.US(ctx, args[1:])
	case "fund":
		result, err = sdk.Quotes.Fund(ctx, args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown market:", args[0])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

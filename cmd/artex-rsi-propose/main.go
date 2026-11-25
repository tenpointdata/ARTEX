// Command artex-rsi-propose is the RSI Loop C proposer (RSI.md rule 6). It
// reads closed live tasks' outcomes, lessons and the skill-gap report from an
// ARTEX server, drafts prompt-body edits for goals/planner/worker that are
// expected to transfer, and writes them as candidate arm files with
// proposed_from set to the tasks they came from. With --post it registers them
// as proposals, inside the round budget GET /api/rsi/status reports.
//
// It never adopts: adoption is an owner action with the owner's password.
// It runs only while the server's ARTEX_RSI_LOOP flag is on.
//
//	ARTEX_TOKEN=... artex-rsi-propose [--artex-url URL] [--out DIR] [--round N] [--post [--only NAME,...]]
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	opt := options{}
	fs := flag.NewFlagSet("artex-rsi-propose", flag.ExitOnError)
	base := fs.String("artex-url", envOr("ARTEX_BASE", "http://localhost:8787"), "ARTEX server URL")
	fs.StringVar(&opt.Out, "out", "rsi-proposals", "directory for candidate arm files (round-<N>/ is appended)")
	fs.IntVar(&opt.Round, "round", -1, "round to propose in; default the newest open round. The next round must be named explicitly")
	fs.IntVar(&opt.MaxTasks, "max-tasks", 50, "newest closed tasks to read")
	fs.BoolVar(&opt.Post, "post", false, "register the drafts as proposals (default: dry run, files only)")
	only := fs.String("only", "", "with --post, register only these drafts (comma-separated arm names, e.g. r0-worker-precision)")
	_ = fs.Parse(os.Args[1:])
	for _, n := range strings.Split(*only, ",") {
		if n = strings.TrimSpace(n); n != "" {
			opt.Only = append(opt.Only, n)
		}
	}
	token := os.Getenv("ARTEX_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "need ARTEX_TOKEN")
		os.Exit(2)
	}
	res, err := run(newClient(*base, token), opt, os.Stderr)
	if res != nil {
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "artex-rsi-propose:", err)
		if errors.Is(err, ErrLoopOff) {
			os.Exit(3)
		}
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

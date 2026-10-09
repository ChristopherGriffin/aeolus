// Command aeolus is the manager.
//
//	aeolus init         create the Org, its first admin and optionally the
//	                    MCP adapter's account, on the manager host (once)
//	aeolus serve        serve the API over HTTPS
//	aeolus token        issue a new token for an account, on the manager
//	                    host, with the service stopped (0043)
//	aeolus fingerprint  print the TLS certificate's SHA-256, which a person
//	                    checks when joining an AP by hand (0083)
//	aeolus version      print the version
package main

import (
	"fmt"
	"io"
	"os"
)

// version is set at build time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "aeolus:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return usage(stderr)
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stderr)
	case "token":
		return runToken(args[1:], stdout, stderr)
	case "fingerprint":
		return runFingerprint(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, version)
		return nil
	}
	return usage(stderr)
}

func usage(w io.Writer) error {
	fmt.Fprintln(w, "usage: aeolus init|serve|token|fingerprint|version [flags]")
	return fmt.Errorf("unknown or missing command")
}

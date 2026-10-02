package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/api"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
)

// runToken issues a new token for an account, on the manager host: the way
// back in when a token is lost (0043). Access to the host is the trust, as
// for aeolus init. The change is logged under the account itself, which may
// always issue and revoke its own tokens (0030).
//
// The service must be stopped while it runs: the change log admits one
// process at a time.
func runToken(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	fs.SetOutput(stderr)
	db := fs.String("db", "/var/lib/aeolus/aeolus.db", "change log database")
	account := fs.String("account", "", "the account to issue a token for")
	revokeOthers := fs.Bool("revoke-others", false, "also revoke the account's other tokens, e.g. a lost one")
	reason := fs.String("reason", "issued on the manager host (aeolus token)", "why, for the change log")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *account == "" {
		return errors.New("token needs -account")
	}
	if err := checkOwner(*db); err != nil {
		return err
	}
	sch, err := schema.V1()
	if err != nil {
		return err
	}
	log, err := changelog.Open(*db, changelog.Options{Check: api.Check(sch)})
	if err != nil {
		return err
	}
	defer log.Close()
	state := log.Snapshot()
	if state == nil {
		return errors.New("the change log holds no Org yet; run aeolus init")
	}
	acct := access.AccountID(*account)
	if _, ok := state.Access.Account(acct); !ok {
		return fmt.Errorf("no account %q", *account)
	}
	others := state.Access.TokensOf(acct)

	as := func(op change.Op) error {
		_, err := log.Commit(*account, *reason, op)
		return err
	}
	plain, err := issue(as, acct)
	if err != nil {
		return err
	}
	revoked := 0
	if *revokeOthers {
		for _, t := range others {
			if t.Revoked {
				continue
			}
			if err := as(change.Op{Kind: change.RevokeToken, TokenID: t.ID}); err != nil {
				return fmt.Errorf("the new token is issued, but revoking %s failed: %w", t.ID, err)
			}
			revoked++
		}
	}

	fmt.Fprintf(stdout, "New token for %s (shown once, keep it safe):\n\n  %s\n\n", *account, plain)
	if *revokeOthers {
		fmt.Fprintf(stdout, "Revoked %d other token(s) of %s.\n", revoked, *account)
	}
	fmt.Fprintln(stdout, "Start the service again: systemctl start aeolus")
	return nil
}

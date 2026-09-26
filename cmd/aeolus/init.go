package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/api"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
)

// runInit creates the Org. It runs once, on the manager host, by the person
// who becomes the first admin: every change it makes is logged under that
// admin's name, and the admin's token is printed only to their terminal.
//
// With -mcp-account, it also creates the MCP adapter's account (0028) and
// writes that account's token to a file, never to the terminal, so no token
// passes through a chat.
func runInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	db := fs.String("db", "/var/lib/aeolus/aeolus.db", "change log database")
	keyPath := fs.String("key", "/etc/aeolus/secret.key", "secret key file (created if missing)")
	orgID := fs.String("org", "", "Org ID, e.g. symtus")
	orgName := fs.String("org-name", "", "Org display name")
	admin := fs.String("admin", "", "first admin's account ID; you, running this")
	mcpAccount := fs.String("mcp-account", "", "account for the MCP adapter, e.g. claude (optional)")
	mcpRole := fs.String("mcp-role", "operator", "role for the MCP adapter at both roots: viewer, operator or admin")
	mcpToken := fs.String("mcp-token", "/etc/aeolus/mcp.token", "file to write the MCP adapter's token to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *orgID == "" || *admin == "" {
		return errors.New("init needs -org and -admin")
	}
	if *orgName == "" {
		*orgName = *orgID
	}
	role := access.None
	if *mcpAccount != "" {
		var err error
		if role, err = access.ParseRole(*mcpRole); err != nil {
			return err
		}
		if _, err := os.Stat(*mcpToken); err == nil {
			return fmt.Errorf("%s already exists; refusing to overwrite a token file", *mcpToken)
		}
	}

	sch, err := schema.V1()
	if err != nil {
		return err
	}
	if _, err := secret.LoadOrCreate(*keyPath); err != nil {
		return fmt.Errorf("secret key: %w", err)
	}
	log, err := changelog.Open(*db, changelog.Options{Check: api.Check(sch)})
	if err != nil {
		return err
	}
	defer log.Close()
	if log.Snapshot() != nil {
		return errors.New("this change log already holds an Org")
	}

	as := func(op change.Op) error {
		_, err := log.Commit(*admin, "aeolus init", op)
		return err
	}
	root := hierarchy.NodeID(*orgID)
	if err := as(change.Op{Kind: change.CreateOrg, Node: root, Name: *orgName, Account: access.AccountID(*admin)}); err != nil {
		return err
	}
	adminToken, err := issue(as, access.AccountID(*admin))
	if err != nil {
		return err
	}

	if *mcpAccount != "" {
		acct := access.AccountID(*mcpAccount)
		if err := as(change.Op{Kind: change.AddAccount, Account: acct, Name: "MCP adapter (" + *mcpAccount + ")"}); err != nil {
			return err
		}
		for _, tree := range []change.TreeName{change.Locations, change.Services} {
			if err := as(change.Op{Kind: change.GrantRole, Account: acct, Tree: tree, Node: root, Role: role.String()}); err != nil {
				return err
			}
		}
		token, err := issue(as, acct)
		if err != nil {
			return err
		}
		if err := os.WriteFile(*mcpToken, []byte(token+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "MCP adapter account %q is %s at both roots; its token is in %s (not shown).\n", *mcpAccount, role, *mcpToken)
	}

	fmt.Fprintf(stdout, "Org %q created; %s is admin of both trees.\n", *orgID, *admin)
	fmt.Fprintf(stdout, "\nYour API token (shown once, keep it safe):\n\n  %s\n\n", adminToken)
	return nil
}

// issue makes a token for an account and returns its plain text.
func issue(as func(change.Op) error, account access.AccountID) (string, error) {
	plain, id, hash, err := access.NewToken()
	if err != nil {
		return "", err
	}
	return plain, as(change.Op{Kind: change.IssueToken, Account: account, TokenID: id, TokenHash: hash})
}

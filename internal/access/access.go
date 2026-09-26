// Package access holds accounts, API tokens and roles (0024, 0025). It is part
// of the state rebuilt from the change log: every account, token and grant
// arrives through a logged change.
//
// Tokens are stored only as a SHA-256 hash of their secret part. The secret is
// 32 random bytes, so a fast hash is enough; there is nothing to brute-force.
package access

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Role is what an account may do on a folder and below it (0025).
type Role int

const (
	None Role = iota
	Viewer
	Operator
	Admin
)

var roleNames = map[Role]string{None: "none", Viewer: "viewer", Operator: "operator", Admin: "admin"}

func (r Role) String() string { return roleNames[r] }

// ParseRole reads "viewer", "operator" or "admin".
func ParseRole(s string) (Role, error) {
	for r, n := range roleNames {
		if n == s && r != None {
			return r, nil
		}
	}
	return None, fmt.Errorf("%w: %q", ErrBadRole, s)
}

// AccountID identifies an account; it is the actor name in the change log.
type AccountID string

// Account is one actor: a person, an automation, or Claude.
type Account struct {
	ID   AccountID
	Name string
}

// Token is an API token as stored: its ID and the hash of its secret.
type Token struct {
	ID      string
	Account AccountID
	Hash    []byte
	Revoked bool
}

// Grant gives an account a role on a folder in one tree.
type Grant struct {
	Account AccountID
	Tree    string
	Node    hierarchy.NodeID
	Role    Role
}

var (
	ErrExists      = errors.New("already exists")
	ErrNoAccount   = errors.New("no such account")
	ErrNoToken     = errors.New("no such token")
	ErrNoGrant     = errors.New("no such grant")
	ErrBadRole     = errors.New("unknown role")
	ErrBadToken    = errors.New("invalid or revoked token")
	ErrBadTokenArg = errors.New("a token needs an ID and a 32-byte hash")
	ErrRevoked     = errors.New("token is already revoked")
)

// Access is the set of accounts, tokens and grants.
type Access struct {
	accounts map[AccountID]*Account
	tokens   map[string]*Token
	grants   map[Grant]bool
}

// New returns an empty Access.
func New() *Access {
	return &Access{accounts: map[AccountID]*Account{}, tokens: map[string]*Token{}, grants: map[Grant]bool{}}
}

// Clone returns an independent copy.
func (a *Access) Clone() *Access {
	c := New()
	for id, acc := range a.accounts {
		cp := *acc
		c.accounts[id] = &cp
	}
	for id, tok := range a.tokens {
		cp := *tok
		cp.Hash = append([]byte(nil), tok.Hash...)
		c.tokens[id] = &cp
	}
	for g := range a.grants {
		c.grants[g] = true
	}
	return c
}

// AddAccount creates an account.
func (a *Access) AddAccount(id AccountID, name string) error {
	if id == "" {
		return fmt.Errorf("%w: empty account ID", ErrNoAccount)
	}
	if _, ok := a.accounts[id]; ok {
		return fmt.Errorf("account %s %w", id, ErrExists)
	}
	a.accounts[id] = &Account{ID: id, Name: name}
	return nil
}

// Account returns an account.
func (a *Access) Account(id AccountID) (Account, bool) {
	acc, ok := a.accounts[id]
	if !ok {
		return Account{}, false
	}
	return *acc, true
}

// AddToken records a token's ID and hash for an account.
func (a *Access) AddToken(id string, account AccountID, hash []byte) error {
	if id == "" || len(hash) != sha256.Size {
		return ErrBadTokenArg
	}
	if _, ok := a.accounts[account]; !ok {
		return fmt.Errorf("%w: %s", ErrNoAccount, account)
	}
	if _, ok := a.tokens[id]; ok {
		return fmt.Errorf("token %s %w", id, ErrExists)
	}
	a.tokens[id] = &Token{ID: id, Account: account, Hash: append([]byte(nil), hash...)}
	return nil
}

// Token returns a stored token.
func (a *Access) Token(id string) (Token, bool) {
	tok, ok := a.tokens[id]
	if !ok {
		return Token{}, false
	}
	return *tok, true
}

// RevokeToken revokes a token for good.
func (a *Access) RevokeToken(id string) error {
	tok, ok := a.tokens[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoToken, id)
	}
	if tok.Revoked {
		return ErrRevoked
	}
	tok.Revoked = true
	return nil
}

// Grant gives a role. The caller checks that the folder exists in the tree.
func (a *Access) Grant(g Grant) error {
	if g.Role == None || roleNames[g.Role] == "" {
		return ErrBadRole
	}
	if _, ok := a.accounts[g.Account]; !ok {
		return fmt.Errorf("%w: %s", ErrNoAccount, g.Account)
	}
	a.grants[g] = true
	return nil
}

// Revoke removes a grant.
func (a *Access) Revoke(g Grant) error {
	if !a.grants[g] {
		return ErrNoGrant
	}
	delete(a.grants, g)
	return nil
}

// Grants lists every grant, sorted.
func (a *Access) Grants() []Grant {
	out := make([]Grant, 0, len(a.grants))
	for g := range a.grants {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if x.Account != y.Account {
			return x.Account < y.Account
		}
		if x.Tree != y.Tree {
			return x.Tree < y.Tree
		}
		if x.Node != y.Node {
			return x.Node < y.Node
		}
		return x.Role < y.Role
	})
	return out
}

// RoleAt returns an account's strongest role on a node, given the node's
// ancestry in a tree (root first). Roles flow down, through breaks (0025).
func (a *Access) RoleAt(account AccountID, tree string, ancestry []hierarchy.NodeID) Role {
	best := None
	for g := range a.grants {
		if g.Account != account || g.Tree != tree || g.Role <= best {
			continue
		}
		for _, n := range ancestry {
			if n == g.Node {
				best = g.Role
				break
			}
		}
	}
	return best
}

// AdminAnywhere reports whether an account is admin on at least one folder.
func (a *Access) AdminAnywhere(account AccountID) bool {
	for g := range a.grants {
		if g.Account == account && g.Role == Admin {
			return true
		}
	}
	return false
}

const tokenPrefix = "aeolus1"

// NewToken makes a token: the plain text to hand out once, and the ID and hash
// to record.
func NewToken() (plain, id string, hash []byte, err error) {
	idb := make([]byte, 8)
	sec := make([]byte, 32)
	if _, err = rand.Read(idb); err != nil {
		return "", "", nil, err
	}
	if _, err = rand.Read(sec); err != nil {
		return "", "", nil, err
	}
	id = hex.EncodeToString(idb)
	secret := base64.RawURLEncoding.EncodeToString(sec)
	sum := sha256.Sum256([]byte(secret))
	return tokenPrefix + "." + id + "." + secret, id, sum[:], nil
}

// Authenticate returns the account a plain token belongs to.
func (a *Access) Authenticate(plain string) (AccountID, error) {
	parts := strings.SplitN(plain, ".", 3)
	if len(parts) != 3 || parts[0] != tokenPrefix {
		return "", ErrBadToken
	}
	tok, ok := a.tokens[parts[1]]
	if !ok || tok.Revoked {
		return "", ErrBadToken
	}
	sum := sha256.Sum256([]byte(parts[2]))
	if subtle.ConstantTimeCompare(sum[:], tok.Hash) != 1 {
		return "", ErrBadToken
	}
	if _, ok := a.accounts[tok.Account]; !ok {
		return "", ErrBadToken
	}
	return tok.Account, nil
}

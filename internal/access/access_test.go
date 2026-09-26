package access

import (
	"errors"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

func TestTokensAuthenticateUntilRevoked(t *testing.T) {
	a := New()
	must(t, a.AddAccount("claude", "Claude"))
	plain, id, hash, err := NewToken()
	must(t, err)
	must(t, a.AddToken(id, "claude", hash))

	if who, err := a.Authenticate(plain); err != nil || who != "claude" {
		t.Fatalf("Authenticate = %q, %v", who, err)
	}
	for _, bad := range []string{
		"", "nope", strings.Replace(plain, "aeolus1", "aeolus2", 1),
		plain[:len(plain)-2] + "xx", "aeolus1." + id + ".wrongsecret",
	} {
		if _, err := a.Authenticate(bad); !errors.Is(err, ErrBadToken) {
			t.Errorf("Authenticate(%q) = %v, want ErrBadToken", bad, err)
		}
	}
	must(t, a.RevokeToken(id))
	if _, err := a.Authenticate(plain); !errors.Is(err, ErrBadToken) {
		t.Fatalf("revoked token still works: %v", err)
	}
}

func TestAPTokens(t *testing.T) {
	a := New()
	plain, id, hash, err := NewAPToken()
	must(t, err)
	if !strings.HasPrefix(plain, "aeolusap1."+id+".") {
		t.Fatalf("AP token %q", plain)
	}
	must(t, a.AddAPToken(id, "ap-1", hash))
	if err := a.AddAPToken(id, "ap-2", hash); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate AP token ID: %v", err)
	}
	c := a.Clone()
	if ap, err := a.AuthenticateAP(plain); err != nil || ap != "ap-1" {
		t.Fatalf("AuthenticateAP = %q, %v", ap, err)
	}
	if _, err := a.AuthenticateAP(strings.Replace(plain, "aeolusap1.", "aeolus1.", 1)); !errors.Is(err, ErrBadToken) {
		t.Fatalf("an AP token with an account prefix: %v", err)
	}
	if n := a.RevokeAPTokens("ap-1"); n != 1 {
		t.Fatalf("revoked %d", n)
	}
	if _, err := a.AuthenticateAP(plain); !errors.Is(err, ErrBadToken) {
		t.Fatalf("after revoke: %v", err)
	}
	if _, err := c.AuthenticateAP(plain); err != nil {
		t.Fatalf("revoking reached a clone: %v", err)
	}
}

func TestStoredTokenHoldsNoSecret(t *testing.T) {
	plain, id, hash, err := NewToken()
	must(t, err)
	secret := strings.SplitN(plain, ".", 3)[2]
	if strings.Contains(string(hash), secret) || strings.Contains(id, secret) {
		t.Fatal("stored parts contain the secret")
	}
}

func TestRolesFlowDownTheAncestry(t *testing.T) {
	a := New()
	must(t, a.AddAccount("office", "Leasing office"))
	must(t, a.Grant(Grant{Account: "office", Tree: "services", Node: "residents", Role: Operator}))
	must(t, a.Grant(Grant{Account: "office", Tree: "services", Node: "symtus", Role: Viewer}))

	under := []hierarchy.NodeID{"symtus", "residents", "building-b"}
	if r := a.RoleAt("office", "services", under); r != Operator {
		t.Fatalf("role under residents = %v, want operator", r)
	}
	if r := a.RoleAt("office", "services", []hierarchy.NodeID{"symtus", "guest"}); r != Viewer {
		t.Fatalf("role elsewhere = %v, want viewer", r)
	}
	if r := a.RoleAt("office", "locations", under); r != None {
		t.Fatalf("role in the other tree = %v, want none", r)
	}
}

func TestGrantAndRevokeRules(t *testing.T) {
	a := New()
	if err := a.Grant(Grant{Account: "ghost", Tree: "locations", Node: "symtus", Role: Admin}); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("grant to unknown account: %v", err)
	}
	must(t, a.AddAccount("griff", "Griff"))
	if err := a.Grant(Grant{Account: "griff", Tree: "locations", Node: "symtus", Role: None}); !errors.Is(err, ErrBadRole) {
		t.Fatalf("grant of no role: %v", err)
	}
	g := Grant{Account: "griff", Tree: "locations", Node: "symtus", Role: Admin}
	must(t, a.Grant(g))
	if !a.AdminAnywhere("griff") {
		t.Fatal("AdminAnywhere false after admin grant")
	}
	must(t, a.Revoke(g))
	if err := a.Revoke(g); !errors.Is(err, ErrNoGrant) {
		t.Fatalf("second revoke: %v", err)
	}
	if err := a.AddAccount("griff", "again"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate account: %v", err)
	}
}

func TestParseRole(t *testing.T) {
	for _, s := range []string{"viewer", "operator", "admin"} {
		r, err := ParseRole(s)
		if err != nil || r.String() != s {
			t.Errorf("ParseRole(%s) = %v, %v", s, r, err)
		}
	}
	for _, s := range []string{"none", "root", ""} {
		if _, err := ParseRole(s); !errors.Is(err, ErrBadRole) {
			t.Errorf("ParseRole(%q) = %v", s, err)
		}
	}
}

func TestCloneIsIndependent(t *testing.T) {
	a := New()
	must(t, a.AddAccount("griff", "Griff"))
	_, id, hash, err := NewToken()
	must(t, err)
	must(t, a.AddToken(id, "griff", hash))
	c := a.Clone()
	must(t, c.RevokeToken(id))
	must(t, c.AddAccount("claude", "Claude"))
	if tok, _ := a.Token(id); tok.Revoked {
		t.Fatal("revoke on the clone reached the original")
	}
	if _, ok := a.Account("claude"); ok {
		t.Fatal("account added to the clone reached the original")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

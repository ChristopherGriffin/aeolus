// Package mcpadapter serves the manager's API as MCP tools at /mcp (0028,
// 0031).
//
// It is a pass-through: every MCP request must carry the caller's own Aeolus
// token, and the adapter makes its API calls with that same token, through the
// API's HTTP handler in the same process. It holds no credentials and has no
// path to the state except the API (0003), so each MCP client acts, and is
// logged, under its own identity.
package mcpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const instructions = `Aeolus is the manager for an OpenWrt-based Wi-Fi system. It holds what each AP should run; APs do the rest themselves.

Two trees sit under the Org: "locations" (where APs are: radios, system, ports, and which service folders apply) and "services" (what APs offer: networks and their transports). Settings inherit down each tree field by field; a value shows where it comes from (self, inherited, locked, baseline). Locks stop overrides below; Break Hierarchy starts a new branch.

Every change is one logged entry with your name and a reason. Run preview_change before make_change for anything that re-versions APs, locks, moves or breaks hierarchy. Secrets such as passphrases are sealed; you can set them but never read them back.`

// New returns the /mcp handler. api is the API's handler; the adapter is its
// client.
func New(api http.Handler, version string) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server(client{api: api, auth: r.Header.Get("Authorization")}, version)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Refuse a request without a working token before any MCP handling.
		c := client{api: api, auth: r.Header.Get("Authorization")}
		if _, err := c.call(r.Context(), "GET", "/v1/whoami", nil); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"error":"missing or invalid token"}`)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// client calls the API in-process with one caller's Authorization header.
type client struct {
	api  http.Handler
	auth string
}

// apiError is an error the API returned.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status) }

func (c client) call(ctx context.Context, method, path string, body any) (any, error) {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c.api.ServeHTTP(rec, req)
	var out any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("API returned %d with a body that is not JSON", rec.Code)
	}
	if rec.Code >= 400 {
		msg := rec.Body.String()
		if m, ok := out.(map[string]any); ok {
			if e, ok := m["error"].(string); ok {
				msg = e
			}
		}
		return nil, &apiError{Status: rec.Code, Message: msg}
	}
	return out, nil
}

// Op is a change, as the model writes it.
type Op struct {
	Kind     string   `json:"kind" jsonschema:"one of: add-folder, add-ap, move, set, unset, lock, unlock, break-hierarchy, assign-services, add-account, grant, revoke, revoke-token, set-concentrator, remove-concentrator, set-vni, remove-vni"`
	Tree     string   `json:"tree,omitempty" jsonschema:"locations or services"`
	Node     string   `json:"node,omitempty" jsonschema:"the folder or AP the change targets; for add-folder and add-ap, the new node's ID"`
	Parent   string   `json:"parent,omitempty" jsonschema:"parent folder, for add-folder, add-ap and move"`
	Name     string   `json:"name,omitempty" jsonschema:"display name, for add-folder, add-ap and add-account"`
	Path     string   `json:"path,omitempty" jsonschema:"field path, e.g. radio.5g.width or network.sweet.transport.primary.vlan"`
	Value    any      `json:"value,omitempty" jsonschema:"the field's new value, for set; for set-concentrator the definition {name, address, port, mtu, scope}"`
	Services []string `json:"services,omitempty" jsonschema:"service folder IDs, for assign-services on a Locations node"`
	Account  string   `json:"account,omitempty" jsonschema:"account ID, for add-account, grant and revoke"`
	Role     string   `json:"role,omitempty" jsonschema:"viewer, operator or admin, for grant and revoke"`
	TokenID  string   `json:"token_id,omitempty" jsonschema:"token ID, for revoke-token"`

	Concentrator string `json:"concentrator,omitempty" jsonschema:"library concentrator ID, for the concentrator and VNI kinds"`
	VNI          int    `json:"vni,omitempty" jsonschema:"VNI number, for set-vni and remove-vni (its label goes in name)"`
}

type treeIn struct {
	Tree string `json:"tree" jsonschema:"locations or services"`
}

type nodeIn struct {
	Tree string `json:"tree" jsonschema:"locations or services"`
	Node string `json:"node" jsonschema:"node ID"`
}

type apIn struct {
	AP string `json:"ap" jsonschema:"AP ID in the Locations tree"`
}

type changesIn struct {
	After int64 `json:"after,omitempty" jsonschema:"return changes after this sequence number"`
	Limit int   `json:"limit,omitempty" jsonschema:"how many changes to return, 1 to 1000 (default 100)"`
}

type previewIn struct {
	Op Op `json:"op" jsonschema:"the change to preview"`
}

type changeIn struct {
	Op     Op     `json:"op" jsonschema:"the change to make"`
	Reason string `json:"reason" jsonschema:"why: recorded in the change log with your name"`
}

type empty struct{}

func server(c client, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "aeolus", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "whoami", Description: "Your account and the roles you hold on folders.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/whoami", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_tree", Description: "List the nodes of the locations or services tree that you can view, root first.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in treeIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/trees/"+url.PathEscape(in.Tree), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_node", Description: "One node's page: every field with its value, where it comes from (from, origin), the overrides in effect and below, the locks a break would escape, and the problems (rules its config breaks).", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in nodeIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/trees/"+url.PathEscape(in.Tree)+"/nodes/"+url.PathEscape(in.Node), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_ap_config", Description: "An AP's fully resolved config: Location fields, networks from its service folders, its version, and whether the whole config passes its check.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in apIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/aps/"+url.PathEscape(in.AP)+"/config", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_library", Description: "The Org's library: concentrators (address, port, MTU, the Location folders they may be used at) and their labeled VNIs. Network transports pick a concentrator and a VNI from here.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/library", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_changes", Description: "Read the change log: who changed what, when and why.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in changesIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.After > 0 {
				q.Set("after", fmt.Sprint(in.After))
			}
			if in.Limit > 0 {
				q.Set("limit", fmt.Sprint(in.Limit))
			}
			path := "/v1/changes"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "preview_change", Description: "Run a change with every check (permissions, schema, locks, APs that would stop resolving) and record nothing. Returns the effect, overrides a lock or move would remove, the APs it would re-version, and any whose config would then fail its check.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in previewIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "POST", "/v1/preview", map[string]any{"op": in.Op})
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "make_change", Description: "Make one change. It is logged under your name with your reason. Set values are checked against the field schema and secrets are sealed. Returns the logged change, the APs it re-versioned, and any whose config now fails its check.", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in changeIn) (*mcp.CallToolResult, any, error) {
			if strings.TrimSpace(in.Reason) == "" {
				return nil, nil, fmt.Errorf("a reason is required: it is recorded in the change log")
			}
			out, err := c.call(ctx, "POST", "/v1/changes", map[string]any{"op": in.Op, "reason": in.Reason})
			return nil, out, err
		})
	return s
}

func ptr[T any](v T) *T { return &v }

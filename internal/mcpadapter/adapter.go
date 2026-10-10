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

Every change is one logged entry with your name and the time; a reason is optional, a note for where the change does not speak for itself. Run preview_change before make_change for anything that re-versions APs, locks, moves or breaks hierarchy. Secrets such as passphrases are sealed; you can set them but never read them back.`

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
	Kind     string         `json:"kind" jsonschema:"one of: add-folder, add-ap, remove-ap, move, set, unset, lock, unlock, break-hierarchy, assign-services, add-account, grant, revoke, revoke-token, add-key, set-key, remove-key, add-template, edit-template, set-template, unset-template, remove-template. The library's concentrator kinds are retired (0085)."`
	Tree     string         `json:"tree,omitempty" jsonschema:"locations or services"`
	Node     string         `json:"node,omitempty" jsonschema:"the folder or AP the change targets; for add-folder and add-ap, the new node's ID"`
	Parent   string         `json:"parent,omitempty" jsonschema:"parent folder, for add-folder, add-ap and move; for add-template, the Locations folder (or the Org) it is made at, offered there and below"`
	Name     string         `json:"name,omitempty" jsonschema:"display name, for add-folder, add-ap, add-account, add-template and edit-template"`
	Path     string         `json:"path,omitempty" jsonschema:"field path, e.g. radio.5g.width or network.sweet.transport.primary.vlan; a kind of AP's own port setting is boards.<board>.ports.<name>.<field>, such as boards.netgear,r7800.ports.lan2.mode (0092)"`
	Value    any            `json:"value,omitempty" jsonschema:"the field's new value, for set; for set-concentrator the definition {name, address, port, mtu, scope}; for add-key and set-key the key {name, passphrase, vlan, macs, expires}, its passphrase in plain text, which the manager seals (set-key may leave it out to keep it)"`
	Values   map[string]any `json:"values,omitempty" jsonschema:"several fields of one node to set together, for set, in place of path and value: {path: value}; all or none are set"`
	Paths    []string       `json:"paths,omitempty" jsonschema:"several fields of one node to unset together, for unset, in place of path; all or none are unset"`
	Services []string       `json:"services,omitempty" jsonschema:"service folder IDs, for assign-services on a Locations node"`
	Account  string         `json:"account,omitempty" jsonschema:"account ID, for add-account, grant and revoke"`
	Role     string         `json:"role,omitempty" jsonschema:"viewer, operator or admin, for grant and revoke"`
	TokenID  string         `json:"token_id,omitempty" jsonschema:"token ID, for revoke-token"`

	Concentrator string `json:"concentrator,omitempty" jsonschema:"retired: the library's concentrators take no new changes (0085)"`
	VNI          int    `json:"vni,omitempty" jsonschema:"retired, with the library's concentrators (0085)"`

	Template string   `json:"template,omitempty" jsonschema:"for the template kinds: the AP template's ID (0085); for set-template and unset-template its fields go in path and value, values or paths, as for set and unset, and add-template may carry values to make it with its settings (0090); and are Locations fields of radio, ports, system (not system.agent), rrm and apc. A folder picks a template for a board with set, path templates.<board>, value the template's ID"`
	Boards   []string `json:"boards,omitempty" jsonschema:"for add-template and edit-template: the boards it is for, as OpenWrt names them (arista,c360)"`
	Default  bool     `json:"default,omitempty" jsonschema:"for add-template: also pick it where it is made, for each of its boards nothing is picked for there"`

	Network string `json:"network,omitempty" jsonschema:"for the key kinds: the network, as the Services folder in node offers it"`
	Key     string `json:"key,omitempty" jsonschema:"for the key kinds: the key's ID; add-key makes one if it is left out"`
}

type journeyIn struct {
	MAC   string `json:"mac" jsonschema:"the client's MAC, such as aa:bb:cc:00:11:22"`
	Hours int    `json:"hours,omitempty" jsonschema:"optional: how many hours back, 24 unless set, at most 720"`
}

type alertsIn struct {
	Under string `json:"under,omitempty" jsonschema:"optional: a Locations folder or AP; only the APs below it"`
}

type libraryIn struct {
	At string `json:"at,omitempty" jsonschema:"a Locations node: list the templates offered there"`
}

type keysIn struct {
	Folder  string `json:"folder" jsonschema:"the Services folder that offers the network"`
	Network string `json:"network" jsonschema:"the network's ID"`
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

type actionIn struct {
	AP   string `json:"ap" jsonschema:"AP ID in the Locations tree"`
	Kind string `json:"kind,omitempty" jsonschema:"locate, restart-wifi or reboot; empty to list its latest actions"`
}

type changeIn struct {
	Op     Op     `json:"op" jsonschema:"the change to make"`
	Reason string `json:"reason,omitempty" jsonschema:"optional: a note for the change log, which records who made the change and when without one"`
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
	mcp.AddTool(s, &mcp.Tool{Name: "get_ap_config", Description: "An AP's fully resolved config: Location fields, networks from its service folders, its version, and whether the whole config passes its check. Its condition says when it was last seen and from where, the version it last reported running and whether that is current (in_sync), and its latest render check, apply and state report.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in apIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/aps/"+url.PathEscape(in.AP)+"/config", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_ap_history", Description: "An AP's recent render checks (with their problems), apply attempts (and whether a passing check covered them) and state reports, newest first.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in apIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/aps/"+url.PathEscape(in.AP)+"/history", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_library", Description: "The Org's library: AP templates (0085). A template is a named set of Locations settings for the APs of one or more boards (arista,c360), made at the Org or a folder and offered there and below. A folder picks one for a board with templates.<board>; an AP takes the one picked nearest above it, its values counting as set where it is picked, ahead of that folder's own, so what is set below or locked replaces them. With at, the templates offered at that Locations node, nearest first. Each lists the APs that take it and whether each follows it; an AP's config (get_ap_config) says which of its template's fields something else replaces. Tunnels are not in the library: they are Location fields, concentrators.<name> (0055).", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in libraryIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/library"
			if in.At != "" {
				path += "?at=" + url.QueryEscape(in.At)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_relayed_dhcp", Description: "What the manager hears from DHCP relays that copy it clients' requests (0068): for each subnet, by the relay's address on it, the relay, the requests of the last 10 minutes, the clients new in that time, a burst of new ones (a sign of DHCP starvation), and its clients newest first, each with its host name, vendor class, parameter list, address, option 82 IDs, maker and the manager's guess at what it is. Relays copy only requests, not servers' answers.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/dhcp/relayed", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_alerts", Description: "What needs attention on the APs you can view (0099), most urgent first, with counts by severity: critical (clients are or will be without service: an AP offline, a config refused or put back, a network with no transport, a loop, netifd's lost wireless object), warning (a config held, an agent update rolled back, a tunnel down, a network on its fallback, a VLAN silent on the uplink, DHCP that does not answer, an AP behind its version) and info (waiting in Landing Zone, a clock not synced). With under, only the APs below that Locations node. Worked out from what the manager has on each call, so an alert ends when its cause does.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in alertsIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/alerts"
			if in.Under != "" {
				path += "?under=" + url.QueryEscape(in.Under)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_airspace", Description: "The Wi-Fi networks the APs you can view hear (0106), once each by BSSID, with its SSID, band and channel, and every AP that hears it with its signal and when it last did, the strongest first. Each is a rogue (it broadcasts one of Aeolus's SSIDs from a BSSID no AP names as its own: an evil twin, or a same-named AP Aeolus does not manage), known (such a BSSID a folder's rogues.known says is no rogue; set it with make_change to quiet one), aeolus (one of the APs' own, heard by an AP that is not its radio neighbour) or other; rogues first. Only APs with radio resource management on listen. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in alertsIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/airspace"
			if in.Under != "" {
				path += "?under=" + url.QueryEscape(in.Under)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_client_journey", Description: "One Wi-Fi client's history across the APs you can view (0103), by its MAC, over the last hours (24 unless set, at most 720): its sessions, each on one AP, band and network, from and to, with its signal (min, max, mean), mean rate, retries, address and DHCP verdict; how often it roamed; and issues: a weak signal, many retries, a slow rate, no DHCP or a static address, or moving back and forth between two APs. Built from the APs' state reports, every few minutes, so a session's edges are to within a report.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in journeyIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/clients/" + url.PathEscape(in.MAC)
			if in.Hours > 0 {
				path += "?hours=" + fmt.Sprint(in.Hours)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_detected", Description: "Devices that may be unconfigured OpenWiFi APs (0034, 0068): possible when a relayed DHCP request asks for options 138 and 224, confirmed when the device knocked on the manager's option 224 listener; and the knocks no device could be matched to by address.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "GET", "/v1/detected", nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_keys", Description: "A network's per-user keys (0070): each its own passphrase on a shared WPA2-PSK network, and the VLAN its client lands in. Lists each key's ID, name, VLAN, the MACs it is bound to and its expiry; passphrases are sealed and never shown. Keys are made, changed and removed with make_change (add-key, set-key, remove-key), and re-version no AP.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in keysIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{"folder": {in.Folder}, "network": {in.Network}}
			out, err := c.call(ctx, "GET", "/v1/keys?"+q.Encode(), nil)
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
	mcp.AddTool(s, &mcp.Tool{Name: "ap_action", Description: "Ask an AP to do something once (0104): locate (blink every LED for a minute, to find it), restart-wifi (every client drops and joins again) or reboot (off the air for about two minutes). It needs operator on the AP. The AP takes it up on its next poll, within about a minute; one it does not take up within 10 minutes expires. A second ask of the same kind while one waits is the same action. Without kind, lists the AP's latest actions and what came of each.", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in actionIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/aps/" + url.PathEscape(in.AP) + "/actions"
			if in.Kind == "" {
				out, err := c.call(ctx, "GET", path, nil)
				return nil, out, err
			}
			out, err := c.call(ctx, "POST", path, map[string]any{"kind": in.Kind})
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "make_change", Description: "Make one change. It is logged under your name with the time, and with your reason if you give one. Set values are checked against the field schema and secrets are sealed. Returns the logged change, the APs it re-versioned, and any whose config now fails its check.", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in changeIn) (*mcp.CallToolResult, any, error) {
			out, err := c.call(ctx, "POST", "/v1/changes", map[string]any{"op": in.Op, "reason": strings.TrimSpace(in.Reason)})
			return nil, out, err
		})
	return s
}

func ptr[T any](v T) *T { return &v }

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
	Kind     string         `json:"kind" jsonschema:"one of: add-folder, remove-folder, add-ap, remove-ap, move, set, unset, lock, unlock, break-hierarchy, assign-services, add-account, grant, revoke, revoke-token, add-key, set-key, remove-key, add-template, edit-template, set-template, unset-template, remove-template. The library's concentrator kinds are retired (0085)."`
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

type clientsIn struct {
	Under  string `json:"under,omitempty" jsonschema:"optional: a Locations folder or AP; only clients last seen on the APs below it"`
	Find   string `json:"find,omitempty" jsonschema:"optional: part of a MAC, a host name, a user or an address"`
	Limit  int    `json:"limit,omitempty" jsonschema:"optional: how many, 100 unless set, at most 500"`
	Offset int    `json:"offset,omitempty" jsonschema:"optional: how many to pass over, to page through"`
}

type connectionsIn struct {
	MAC    string `json:"mac" jsonschema:"the client's MAC, such as aa:bb:cc:00:11:22"`
	Limit  int    `json:"limit,omitempty" jsonschema:"optional: how many attempts, 50 unless set, at most 200"`
	Before string `json:"before,omitempty" jsonschema:"optional: an attempt's started time, to page back from"`
}

type usageIn struct {
	Under string `json:"under,omitempty" jsonschema:"optional: a Locations folder or AP; only the APs below it"`
	Hours int    `json:"hours,omitempty" jsonschema:"optional: how many hours back, 24 unless set, at most 720"`
}

type roundIn struct {
	Under string `json:"under" jsonschema:"a Locations folder or AP: every AP at or below it tests each thing it can once, now"`
	Over  string `json:"over,omitempty" jsonschema:"optional: which tests (0131): air, through the air with a scan radio; wire, over the wire from the uplink; or both, which it is unless said"`
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
	AP       string   `json:"ap" jsonschema:"AP ID in the Locations tree"`
	Kind     string   `json:"kind,omitempty" jsonschema:"locate, restart-wifi, reboot, disconnect, client-test or remove-network; empty to list its latest actions"`
	Target   string   `json:"target,omitempty" jsonschema:"for disconnect: the client's MAC"`
	Network  string   `json:"network,omitempty" jsonschema:"for client-test: the network to join, by its ID, one the AP offers"`
	On       string   `json:"on,omitempty" jsonschema:"for client-test: the AP whose networks to join, the testing AP itself among them; empty for the strongest other AP it hears"`
	Band     string   `json:"band,omitempty" jsonschema:"for client-test: 2g, 5g or 6g; empty for any; wire for a test over the wire, from the AP itself"`
	Remote   *bool    `json:"remote,omitempty" jsonschema:"for client-test: false to leave out the speed to the remote server, which moves a good deal over the internet link; measured where one is set unless false"`
	SSID     string   `json:"ssid,omitempty" jsonschema:"for remove-network: the SSID of the AP's own network to take out"`
	Sections []string `json:"sections,omitempty" jsonschema:"for remove-network: the AP's names for that network in its config, one a radio, as its state report's others has them"`
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
	mcp.AddTool(s, &mcp.Tool{Name: "list_alerts", Description: "What needs attention on the APs you can view (0099), most urgent first, with counts by severity: critical (clients are or will be without service: an AP offline, a config refused or put back, a network with no transport, a loop, netifd's lost wireless object), warning (a config held, an agent update rolled back, a tunnel down, a network on its fallback, a VLAN silent on the uplink, DHCP that does not answer, an AP behind its version, a network that failed its last two client tests on an AP) and info (waiting in Landing Zone, a clock not synced). With under, only the APs below that Locations node. Worked out from what the manager has on each call, so an alert ends when its cause does.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in alertsIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/alerts"
			if in.Under != "" {
				path += "?under=" + url.QueryEscape(in.Under)
			}
			out, err := c.call(ctx, "GET", path, nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_assurance", Description: "Service assurance (0123): the client tests (0121) of the APs you can view that can test, through the air with a scan radio or over the wire (0128), over the last hours (24 unless set, at most 720). testers: each such AP, how often it is asked for a test on a schedule (every, in minutes; 0 for never; set with make_change on assurance.tests.every at a folder, assurance.tests.speed for a measurement against the local speed server in them and assurance.tests.speed_remote for one against the remote server (0134), and assurance.tests.over for which tests, 0131: air, through the air by an AP with a scan radio, which it is unless set; wire, over the wire by every AP below, a scan radio or none, each of its networks, 0128; or both. A tester says which in over, has wire true where it is asked for tests over the wire and can make them, and what it tests over the wire has wire for its band) and how many things it tests in turn. checks: each thing tested, a network on an AP on a band, by which AP, with how many of its tests passed and failed, its latest, and whether it is failing (its last two failed, which is also an alert on the AP it failed on), the failing first; way is what its tests' ways online come to, a point at a time (0130): at each point how many passed it, how many stopped there, and the middle (ms), least and most of their times in ms. A test's own detail.way is its points in order, each ok, bad, begun or none, with how long it took. tests: the latest 50, newest first, each as ap_action lists it. round: a round of every test asked by hand that the APs are in (ask_assurance_round, 0129), with which tests it is of (over: air, wire or both, 0131), how many things that is, and how many were asked for and are done since. joins: how the clients' own attempts to come online went over the same time (0127), a network each, on every AP you can view, scan radio or none: how many clients tried, got on (online) and did not (failed), each counted by the latest of its attempts to come to anything, how long one typically took to come online (typical_ms), where the failed attempts stopped and why (stops, the most clients first), each AP's part (aps), the attempts' way online a point at a time as a check's is (way, over way_of attempts), and failing where three clients or more were let on at an AP in the last hour and got no further (no address, no gateway, no DNS), being half or more of those let on there, which is the alert joins-failing on that AP. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/assurance?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_roaming", Description: "Roaming (0136): the clients that moved from one AP to another, or from one radio of an AP to another, on the APs you can view, over the last hours (24 unless set, at most 720); with under, the APs below that Locations node. A roam is a client's attempt to come online at an AP other than the one it was last let on at, where it came as one already on the network (a reassociation) or by a fast transition, or where it was on that other, on the same network, minutes before; kind full is a hard roam, the sign-in or WPA3's exchange again. A move from one radio of an AP to another of the same is a band switch (band_switch on its row), counted in band_switches and in no other count. roams, failed (not let on), slow (let on after more than slow_over_ms, 250), left (given up by the client before it was let on, not counted as roams), clients, untimed (let on, and not timed: their AP's log has the association and no line of the authentication, as on a network that signs clients in), and typical_ms, the middle of the times of the rest, from a client's authentication at the AP it came to, as its log has it, until it was let on. kinds: the same a kind at a time, with low_ms and high_ms: ft (802.11r, the keys come with it), okc (a key made from its sign-in at another AP), pmksa (a key the AP held from its last time there), cached (one of those two, on a network that signs clients in, where the AP does not say which), keys (a passphrase: the four key messages) and full (the sign-in or WPA3's exchange again). latest: the latest 50, each {id, at, mac, host, ap, ap_name, network, ssid, band, from_ap, from_ap_name, from_band, kind, ms, untimed, state, reason, sa_queries}, state being ok, slow, failed or left, from where the client was last let on before, on any AP, where that is known and yours to view, and sa_queries how often the AP asked a client it still held to prove itself before taking it (802.11w), a second's wait. bad: those of them that are not ok. id is the attempt's, as get_client_connections lists it.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/roaming?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "ask_assurance_round", Description: "Ask every AP below a Locations folder, or one AP, for a round of client tests now (0129): each tests every thing it can, once, one at a time from its next poll, whatever its schedule or none: through the air where it has a scan radio (0121), and over the wire where its agent can (0128), whether a folder asks for that on a schedule or not. over says which tests, 0131: air, wire, or both, which it is unless said. For checking a site that was just built or changed. It needs operator there, and is refused where no AP there can test. A round takes about a minute a thing for each AP; what is not asked for within the hour is let go. Read how it goes with get_assurance: round says how many things, asked and done, and checks and tests what they found.", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in roundIn) (*mcp.CallToolResult, any, error) {
			body := map[string]any{"under": in.Under}
			if in.Over != "" {
				body["over"] = in.Over
			}
			out, err := c.call(ctx, "POST", "/v1/assurance/round", body)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_alert_history", Description: "The alerts that lasted on the APs you can view (0109), at some time in the last hours (24 unless set, at most 720), newest first, at most 500: each with its AP, severity, kind, key and message, when it began, and when it ended, or none while it lasts. An alert is logged once it has lasted two minutes, from when it was first seen; a shorter blip is not. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/alerts/history?"+q.Encode(), nil)
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
	mcp.AddTool(s, &mcp.Tool{Name: "get_usage", Description: "The Wi-Fi clients and traffic of the APs you can view (0108), over the last hours (24 unless set, at most 720), in at most 48 buckets: in each, the most clients the APs had at once, summed over the APs, and the bytes they sent their clients (down) and received from them (up); the totals; each AP's most clients and traffic, the busiest first; and the ten clients that moved the most (0110), by MAC and host name, with the APs they used. Worked out from the APs' state reports, every few minutes. With under, only the APs below that Locations node.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in usageIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Hours > 0 {
				q.Set("hours", fmt.Sprint(in.Hours))
			}
			out, err := c.call(ctx, "GET", "/v1/usage?"+q.Encode(), nil)
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
	mcp.AddTool(s, &mcp.Tool{Name: "list_clients", Description: "Every Wi-Fi client ever seen whose last AP you can view (0118), the latest first, kept for good: each one's MAC, when it was first and last seen and on which AP and network, the host name it gives, who it signed in as, its address, how many times it tried to come online and how many of those failed, and how its latest attempt went. Find one by part of its MAC, host name, user or address.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in clientsIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Under != "" {
				q.Set("under", in.Under)
			}
			if in.Find != "" {
				q.Set("q", in.Find)
			}
			if in.Limit > 0 {
				q.Set("limit", fmt.Sprint(in.Limit))
			}
			if in.Offset > 0 {
				q.Set("offset", fmt.Sprint(in.Offset))
			}
			out, err := c.call(ctx, "GET", "/v1/clients?"+q.Encode(), nil)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_client_connections", Description: "One Wi-Fi client's attempts to come online, on the APs you can view (0118), by its MAC, the latest first, kept 90 days: each with its AP, network and band, when it began, how it went (online, connected, failed or left), the stage it reached or stopped at (auth, assoc, signin, key, dhcp, gateway, dns, internet) and why, its way online a point at a time (way, 0130: each point ok, bad, begun or none, and how long it took in ms; the keys with parts, the four messages of their handshake m1 to m4, where the AP said them, 0133: a wrong passphrase stops at m2 with said \"wrong passphrase\", a client that does not answer with \"no answer\"), and its record: every step the AP saw, with its time in ms after the start. The steps are hostapd's own words for authentication, association, an 802.1X sign-in and the keys; DHCP's messages with the server, gateway and DNS servers given; the ARP for the gateway and its answer; the first DNS lookups and answers; and the first TCP connection opened and answered. Also where the client was heard asking for networks before it joined, where band steering runs. A failed attempt is here though the client never appears in a state report.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in connectionsIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if in.Limit > 0 {
				q.Set("limit", fmt.Sprint(in.Limit))
			}
			if in.Before != "" {
				q.Set("before", in.Before)
			}
			out, err := c.call(ctx, "GET", "/v1/clients/"+url.PathEscape(in.MAC)+"/connections?"+q.Encode(), nil)
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
	mcp.AddTool(s, &mcp.Tool{Name: "ap_action", Description: "Ask an AP to do something once (0104): locate (blink every LED for a minute, to find it), restart-wifi (every client drops and joins again), reboot (off the air for about two minutes) or disconnect (0107: one client, by MAC in target, leaves every network it is on there and may join again at once, to make it reconnect); or client-test (0121: an AP with a scan radio joins one of its own networks, named in network, as a client would, on the AP named in on or else the strongest other AP it hears, and times each step: finding it, the keys, DHCP, the gateway, DNS and, where a speed server is set, how fast, to the local one (system.speed_server) and to the remote one (system.speed_server_remote, 0134: a public server by its name, as cloudflare, or one's own by its URL; with remote false that one is left out, as on a link where bytes cost); its result is the action's detail, read by listing the AP's actions a minute or two later; with band wire it is made over the wire instead (0128), by any AP whose agent can, a scan radio or none: from its own uplink, on the network's VLAN or into its tunnel, it asks for an address by DHCP, its gateway by ARP and a name by DNS, joins nothing, uses no radio and takes a few seconds); or remove-network (0137: the AP takes one of its own Wi-Fi networks, set on the AP and not by Aeolus, out of its config, named by ssid and by sections, its names for it, one a radio, as the others of its state report has them; its clients are off it for good, the AP keeps what it takes out in /etc/aeolus/removed, and Aeolus has no copy: ask the person before this one). It needs operator on the AP. The AP takes it up on its next poll, within about a minute; one it does not take up within 10 minutes expires. A second ask of the same kind while one waits is the same action. Without kind, lists the AP's latest actions and what came of each.", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in actionIn) (*mcp.CallToolResult, any, error) {
			path := "/v1/aps/" + url.PathEscape(in.AP) + "/actions"
			if in.Kind == "" {
				out, err := c.call(ctx, "GET", path, nil)
				return nil, out, err
			}
			body := map[string]any{"kind": in.Kind}
			if in.Target != "" {
				body["target"] = in.Target
			}
			for k, v := range map[string]string{"network": in.Network, "on": in.On, "band": in.Band} {
				if v != "" {
					body[k] = v
				}
			}
			if in.Remote != nil {
				body["remote"] = *in.Remote
			}
			if in.SSID != "" || len(in.Sections) > 0 {
				body["ssid"], body["sections"] = in.SSID, in.Sections
			}
			out, err := c.call(ctx, "POST", path, body)
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

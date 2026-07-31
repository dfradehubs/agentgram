package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/security"
)

// adminToolPrefix namespaces the administration tools. Disjoint from ask_,
// mcp_, group__ and skill__.
const adminToolPrefix = "admin_"

// auditContentLimit caps the prompt/response text returned by admin_query_audit.
// The audit log holds the conversations of every user, so the MCP surface shows
// enough to identify an interaction and no more — the web admin has the rest.
const auditContentLimit = 500

// adminIDPattern bounds the values interpolated into an admin path. It accepts
// resource IDs and emails while rejecting anything that could escape the
// intended route ("/", "..", control characters).
var adminIDPattern = regexp.MustCompile(`^[A-Za-z0-9._@+-]{1,128}$`)

// Warnings appended to every admin tool description. MCP clients surface the
// description to the model, so this is the main brake against a model deciding
// to reconfigure the instance on its own initiative.
const (
	adminWriteWarning = " WRITE OPERATION — it changes the configuration of this Agentgram " +
		"instance for every user. Do NOT call it unless the user has explicitly asked for this " +
		"specific change in this conversation. Never call it to explore, to verify a hypothesis, " +
		"or as a side effect of another task. Show the user the exact payload and get their " +
		"confirmation before calling. Every call is recorded in the audit log with the caller's identity."

	adminDeleteWarning = " DESTRUCTIVE AND IRREVERSIBLE — the resource and its permissions are " +
		"removed and any user or client depending on it breaks immediately. Only call it when the " +
		"user has explicitly asked to delete this exact resource, and confirm the ID with them " +
		"first. Every call is recorded in the audit log with the caller's identity."

	adminReadWarning = " Read-only, but it exposes instance-wide administrative data. Only call it " +
		"when the user asked for this information; do not call it to gather context for another task."

	agentBodyFields = "Required: id, name, protocol (custom|a2a|adk), endpoint. " +
		"Common optional: description, category, headers, allowed_users, allowed_groups, " +
		"auth_type (none|forward|bearer), bearer_token, auth_header_name, api_key_rules, " +
		"rate_limit, health_check, polling, custom_format, max_context_tokens, summarize_threshold. " +
		"Call admin_get_agent on an existing agent to see the exact shape."

	mcpBodyFields = "Required: id, name, transport (http|sse), url. " +
		"Common optional: description, headers, allowed_users, allowed_groups, " +
		"auth_type (none|forward|bearer|oauth2), bearer_token, auth_header_name, api_key_rules, " +
		"oauth2_auth_server_url, oauth2_client_id, oauth2_client_secret, oauth2_scopes. " +
		"Call admin_get_mcp_server on an existing server to see the exact shape."
)

// adminTool is one administrative operation exposed as an MCP tool. It carries no
// logic of its own: the dispatcher replays it against the admin HTTP router, so
// validation, permission gating, audit and registry reloads stay in the handlers
// that the web admin already uses.
type adminTool struct {
	name        string
	title       string
	method      string
	path        string // may contain a single "{id}" placeholder
	minRole     string
	description string
	schema      map[string]interface{}

	readOnly    bool
	destructive bool
	idempotent  bool

	// query lists the argument names forwarded as query-string parameters.
	query []string
	// bodyArg names the argument marshalled as the request body ("body" for
	// resources, "values" for settings). Empty means no body.
	bodyArg string
	// resolvePath overrides `path` when the route depends on the arguments.
	resolvePath func(args map[string]interface{}) (string, error)
	// transform post-processes the successful response body.
	transform func([]byte) []byte
}

// adminTools is the whole administrative surface of the MCP facade. Adding an
// operation is one row here; nothing else changes.
var adminTools = []adminTool{
	{
		name: "admin_list_agents", title: "List agents (admin view)",
		method: http.MethodGet, path: "/agents", minRole: models.RoleEditor, readOnly: true,
		description: "[ADMIN] List every registered agent with its full admin configuration " +
			"(endpoint, protocol, permissions, auth mode). Credentials are redacted." + adminReadWarning,
		schema: emptySchema(),
	},
	{
		name: "admin_get_agent", title: "Get agent (admin view)",
		method: http.MethodGet, path: "/agents/{id}", minRole: models.RoleEditor, readOnly: true,
		description: "[ADMIN] Get one agent's full admin configuration by ID. Credentials come back " +
			"as \"***\"; sending them back unchanged in admin_update_agent keeps the stored value." + adminReadWarning,
		schema: idSchema("The agent ID"),
	},
	{
		name: "admin_create_agent", title: "Create agent",
		method: http.MethodPost, path: "/agents", minRole: models.RoleEditor, bodyArg: "body",
		description: "[ADMIN] Register a new agent." + adminWriteWarning,
		schema:      bodySchema("The agent definition. " + agentBodyFields),
	},
	{
		name: "admin_update_agent", title: "Update agent",
		method: http.MethodPut, path: "/agents/{id}", minRole: models.RoleEditor, bodyArg: "body",
		idempotent: true, destructive: true,
		description: "[ADMIN] Replace an existing agent's configuration. Read it first with " +
			"admin_get_agent and send the complete object back with your changes applied — omitted " +
			"fields are reset." + adminWriteWarning,
		schema: idBodySchema("The agent ID", "The complete agent definition. "+agentBodyFields),
	},
	{
		name: "admin_delete_agent", title: "Delete agent",
		method: http.MethodDelete, path: "/agents/{id}", minRole: models.RoleAdmin,
		destructive: true, idempotent: true,
		description: "[ADMIN] Delete an agent and its permissions." + adminDeleteWarning,
		schema:      idSchema("The agent ID"),
	},
	{
		name: "admin_list_mcp_servers", title: "List MCP servers (admin view)",
		method: http.MethodGet, path: "/mcp", minRole: models.RoleEditor, readOnly: true,
		description: "[ADMIN] List every registered MCP server with its full admin configuration " +
			"(transport, URL, permissions, auth mode). Credentials are redacted." + adminReadWarning,
		schema: emptySchema(),
	},
	{
		name: "admin_get_mcp_server", title: "Get MCP server (admin view)",
		method: http.MethodGet, path: "/mcp/{id}", minRole: models.RoleEditor, readOnly: true,
		description: "[ADMIN] Get one MCP server's full admin configuration by ID. Credentials come " +
			"back as \"***\"; sending them back unchanged in admin_update_mcp_server keeps the stored " +
			"value." + adminReadWarning,
		schema: idSchema("The MCP server ID"),
	},
	{
		name: "admin_create_mcp_server", title: "Create MCP server",
		method: http.MethodPost, path: "/mcp", minRole: models.RoleEditor, bodyArg: "body",
		description: "[ADMIN] Register a new MCP server." + adminWriteWarning,
		schema:      bodySchema("The MCP server definition. " + mcpBodyFields),
	},
	{
		name: "admin_update_mcp_server", title: "Update MCP server",
		method: http.MethodPut, path: "/mcp/{id}", minRole: models.RoleEditor, bodyArg: "body",
		idempotent: true, destructive: true,
		description: "[ADMIN] Replace an existing MCP server's configuration. Read it first with " +
			"admin_get_mcp_server and send the complete object back with your changes applied — " +
			"omitted fields are reset." + adminWriteWarning,
		schema: idBodySchema("The MCP server ID", "The complete MCP server definition. "+mcpBodyFields),
	},
	{
		name: "admin_delete_mcp_server", title: "Delete MCP server",
		method: http.MethodDelete, path: "/mcp/{id}", minRole: models.RoleAdmin,
		destructive: true, idempotent: true,
		description: "[ADMIN] Delete an MCP server and its permissions." + adminDeleteWarning,
		schema:      idSchema("The MCP server ID"),
	},
	{
		name: "admin_query_audit", title: "Query audit log",
		method: http.MethodGet, path: "/audit", minRole: models.RoleAdmin, readOnly: true,
		query:     []string{"from", "to", "user", "group", "resource_type", "session", "limit", "offset"},
		transform: truncateAuditContent,
		description: fmt.Sprintf("[ADMIN] Query the audit log: who used which agent, MCP server, skill "+
			"or group, when, and whether it failed. Use resource_type=\"admin\" to list configuration "+
			"changes instead of conversations. Prompt and response text is truncated to %d characters "+
			"here — the web admin shows it in full."+adminReadWarning, auditContentLimit),
		schema: auditSchema(),
	},
	{
		name: "admin_metrics", title: "Query observability metrics",
		method: http.MethodGet, minRole: models.RoleAdmin, readOnly: true,
		query:       []string{"from", "to", "interval", "limit", "source"},
		resolvePath: resolveMetricsPath,
		description: "[ADMIN] Query aggregated observability metrics (request counts, error rate, " +
			"latency p95, token usage, timelines, rankings) for the whole instance, one user, or one " +
			"agent/MCP server. Contains no conversation content." + adminReadWarning,
		schema: metricsSchema(),
	},
	{
		name: "admin_get_settings", title: "Get global settings",
		method: http.MethodGet, path: "/settings", minRole: models.RoleAdmin, readOnly: true,
		description: "[ADMIN] List the instance-wide runtime settings with their current value, " +
			"default and allowed range." + adminReadWarning,
		schema: emptySchema(),
	},
	{
		name: "admin_update_settings", title: "Update global settings",
		method: http.MethodPut, path: "/settings", minRole: models.RoleAdmin, bodyArg: "values",
		idempotent: true, destructive: true,
		description: "[ADMIN] Change instance-wide runtime settings. These affect every user and " +
			"every conversation immediately. Call admin_get_settings first to see the valid keys and " +
			"ranges." + adminWriteWarning,
		schema: settingsSchema(),
	},
}

// metricsPaths maps a scope/view pair to the admin metrics route that serves it.
// Only the combinations the API actually exposes are present.
var metricsPaths = map[string]string{
	"global/stats":        "/metrics/overview",
	"global/timeline":     "/metrics/overview/timeline",
	"global/top":          "/metrics/overview/top",
	"global/users":        "/metrics/overview/users",
	"global/errors":       "/metrics/overview/errors",
	"global/error_events": "/metrics/overview/error-events",

	"user/stats":    "/metrics/users/{id}",
	"user/timeline": "/metrics/users/{id}/timeline",
	"user/top":      "/metrics/users/{id}/resources",

	"agent/stats":        "/metrics/agents/{id}",
	"agent/timeline":     "/metrics/agents/{id}/timeline",
	"agent/users":        "/metrics/agents/{id}/users",
	"agent/errors":       "/metrics/agents/{id}/errors",
	"agent/error_events": "/metrics/agents/{id}/error-events",

	"mcp/stats":        "/metrics/mcp/{id}",
	"mcp/timeline":     "/metrics/mcp/{id}/timeline",
	"mcp/users":        "/metrics/mcp/{id}/users",
	"mcp/errors":       "/metrics/mcp/{id}/errors",
	"mcp/error_events": "/metrics/mcp/{id}/error-events",
}

// definition renders the tool as an MCP tool descriptor. The annotations are what
// makes a client such as Claude ask the user before running a write tool.
func (t adminTool) definition() map[string]interface{} {
	return map[string]interface{}{
		"name":        t.name,
		"description": t.description,
		"inputSchema": t.schema,
		"annotations": map[string]interface{}{
			"title":           t.title,
			"readOnlyHint":    t.readOnly,
			"destructiveHint": t.destructive,
			"idempotentHint":  t.idempotent,
			"openWorldHint":   false,
		},
	}
}

// adminToolsForRole returns the tools a user with the given effective role may
// use. The admin router enforces the same limit, so this only keeps unusable
// tools out of the client's context.
func adminToolsForRole(role string) []adminTool {
	if role == "" {
		return nil
	}
	var out []adminTool
	for _, t := range adminTools {
		if models.HasRole(role, t.minRole) {
			out = append(out, t)
		}
	}
	return out
}

func lookupAdminTool(name string) (adminTool, bool) {
	for _, t := range adminTools {
		if t.name == name {
			return t, true
		}
	}
	return adminTool{}, false
}

// --- input schemas -----------------------------------------------------------

func emptySchema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func idSchema(desc string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string", "description": desc},
		},
		"required": []string{"id"},
	}
}

// bodySchema describes a create payload. The body stays a free-form object:
// mirroring the admin request DTO field by field would be hundreds of lines of
// schema drifting away from the Go struct.
// ponytail: free-form body + a field list in the description. Upgrade path if a
// model gets the fields wrong: generate the schema from handlers.AdminAgentRequest
// by reflection at startup.
func bodySchema(desc string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"body": objectProperty(desc),
		},
		"required": []string{"body"},
	}
}

func idBodySchema(idDesc, bodyDesc string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id":   map[string]interface{}{"type": "string", "description": idDesc},
			"body": objectProperty(bodyDesc),
		},
		"required": []string{"id", "body"},
	}
}

func objectProperty(desc string) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          desc,
		"additionalProperties": true,
	}
}

func auditSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"from":          map[string]interface{}{"type": "string", "description": "Start of the range, RFC3339 (e.g. 2026-07-01T00:00:00Z)"},
			"to":            map[string]interface{}{"type": "string", "description": "End of the range, RFC3339"},
			"user":          map[string]interface{}{"type": "string", "description": "Filter by user email"},
			"group":         map[string]interface{}{"type": "string", "description": "Filter by one of the caller's groups"},
			"resource_type": map[string]interface{}{"type": "string", "enum": []string{"agent", "mcp", "skill", "group", "admin"}, "description": "Filter by resource type; \"admin\" lists configuration changes"},
			"session":       map[string]interface{}{"type": "string", "description": "Filter by session ID"},
			"limit":         map[string]interface{}{"type": "integer", "description": "Max events to return (default 50, max 1000)"},
			"offset":        map[string]interface{}{"type": "integer", "description": "Pagination offset"},
		},
	}
}

func metricsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"scope": map[string]interface{}{
				"type": "string", "enum": []string{"global", "user", "agent", "mcp"},
				"description": "What to aggregate over: the whole instance, one user, one agent or one MCP server",
			},
			"view": map[string]interface{}{
				"type": "string", "enum": []string{"stats", "timeline", "top", "users", "errors", "error_events"},
				"description": "stats = totals; timeline = bucketed series; top = busiest resources; " +
					"users = per-user breakdown; errors = grouped error types; error_events = raw failures",
			},
			"id":       map[string]interface{}{"type": "string", "description": "Required unless scope is \"global\": the user email, agent ID or MCP server ID"},
			"from":     map[string]interface{}{"type": "string", "description": "Start of the range, RFC3339 (default: last 24h)"},
			"to":       map[string]interface{}{"type": "string", "description": "End of the range, RFC3339"},
			"interval": map[string]interface{}{"type": "string", "enum": []string{"5m", "15m", "30m", "1h", "6h", "1d"}, "description": "Timeline bucket size (default 1h)"},
			"limit":    map[string]interface{}{"type": "integer", "description": "Max rows for top/users views"},
			"source":   map[string]interface{}{"type": "string", "enum": []string{"web", "slack"}, "description": "Restrict to one surface; omit for all"},
		},
		"required": []string{"scope", "view"},
	}
}

func settingsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"values": map[string]interface{}{
				"type": "object",
				"description": "Map of setting key to new value, as strings. Only the keys you pass " +
					"are changed. Use admin_get_settings for the valid keys and their ranges.",
				"additionalProperties": map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"values"},
	}
}

// --- dispatch ----------------------------------------------------------------

// resolveMetricsPath turns the scope/view pair into an admin metrics route.
func resolveMetricsPath(args map[string]interface{}) (string, error) {
	scope := stringArg(args, "scope")
	view := stringArg(args, "view")
	if scope == "" || view == "" {
		return "", fmt.Errorf("scope and view are required")
	}

	path, ok := metricsPaths[scope+"/"+view]
	if !ok {
		return "", fmt.Errorf("unsupported scope/view combination %q/%q. Supported: %s", scope, view, strings.Join(supportedMetricsCombos(), ", "))
	}
	return path, nil
}

func supportedMetricsCombos() []string {
	combos := make([]string, 0, len(metricsPaths))
	for k := range metricsPaths {
		combos = append(combos, k)
	}
	sort.Strings(combos)
	return combos
}

// adminToolResult runs an admin tool and returns the text to send back plus
// whether it is an error result. userAgent identifies the calling MCP client and
// is carried through to the audit entry.
func (h *Handler) adminToolResult(ctx context.Context, tool adminTool, args map[string]interface{}, userAgent string) (string, bool) {
	if h.adminRouter == nil {
		return "The admin API is not enabled on this instance", true
	}

	path := tool.path
	if tool.resolvePath != nil {
		resolved, err := tool.resolvePath(args)
		if err != nil {
			return err.Error(), true
		}
		path = resolved
	}

	path, err := interpolateAdminPath(path, args)
	if err != nil {
		return err.Error(), true
	}

	body, err := adminRequestBody(tool, args)
	if err != nil {
		return err.Error(), true
	}

	// A client that read the resource got "***" for every credential. Restore the
	// stored values before writing, or the update would wipe them.
	if len(body) > 0 && security.HasRedacted(body) && tool.method == http.MethodPut {
		if status, current, err := h.callAdminAPI(ctx, http.MethodGet, path, "", nil, userAgent); err == nil && status == http.StatusOK {
			body = security.RestoreRedacted(body, current)
		}
	}

	status, respBody, err := h.callAdminAPI(ctx, tool.method, path, adminQuery(tool, args), body, userAgent)
	if err != nil {
		return "Failed to call the admin API: " + err.Error(), true
	}

	respBody = security.RedactJSON(respBody)
	if status < 200 || status >= 300 {
		return fmt.Sprintf("Admin API returned %d: %s", status, strings.TrimSpace(string(respBody))), true
	}

	if tool.transform != nil {
		respBody = tool.transform(respBody)
	}

	text := prettyJSON(respBody)
	if strings.TrimSpace(text) == "" {
		// Deletes answer 204 with no body; an empty tool result reads as a failure.
		text = fmt.Sprintf("Done (HTTP %d, the admin API returned no content)", status)
	}
	return text, false
}

// callAdminAPI replays a request against the admin router in-process.
func (h *Handler) callAdminAPI(ctx context.Context, method, path, query string, body []byte, userAgent string) (int, []byte, error) {
	target := path
	if query != "" {
		target += "?" + query
	}

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(adminLoopbackContext(ctx), method, target, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if userAgent != "" {
		// Keeps the calling MCP client visible in the audit entry.
		req.Header.Set("User-Agent", userAgent)
	}

	rec := &loopbackRecorder{headers: make(http.Header), status: http.StatusOK}
	h.adminRouter.ServeHTTP(rec, req)
	return rec.status, rec.body.Bytes(), nil
}

// adminLoopbackContext prepares the context for the synthesized request. It keeps
// the caller's claims (the admin router's RoleGate reads them) and marks the
// request as MCP-originated for the audit log.
func adminLoopbackContext(ctx context.Context) context.Context {
	// chi's Mux.ServeHTTP reuses an existing RouteContext instead of matching from
	// scratch, and this context already carries the one from the /mcp route.
	// Clearing it makes the admin mux route the synthesized path properly.
	ctx = context.WithValue(ctx, chi.RouteCtxKey, nil)
	return middleware.WithSurfaceMCP(ctx)
}

// interpolateAdminPath substitutes {id} with the validated argument.
func interpolateAdminPath(path string, args map[string]interface{}) (string, error) {
	if !strings.Contains(path, "{id}") {
		return path, nil
	}
	id := stringArg(args, "id")
	if id == "" {
		return "", fmt.Errorf("the \"id\" argument is required")
	}
	if !adminIDPattern.MatchString(id) {
		return "", fmt.Errorf("invalid id %q: only letters, digits and . _ @ + - are allowed", id)
	}
	return strings.ReplaceAll(path, "{id}", id), nil
}

// adminRequestBody marshals the argument the tool sends as its request body.
func adminRequestBody(tool adminTool, args map[string]interface{}) ([]byte, error) {
	if tool.bodyArg == "" {
		return nil, nil
	}
	raw, ok := args[tool.bodyArg]
	if !ok || raw == nil {
		return nil, fmt.Errorf("the %q argument is required", tool.bodyArg)
	}
	if _, ok := raw.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("the %q argument must be an object", tool.bodyArg)
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %q argument: %w", tool.bodyArg, err)
	}
	return body, nil
}

// adminQuery builds the query string from the tool's declared parameters.
func adminQuery(tool adminTool, args map[string]interface{}) string {
	if len(tool.query) == 0 {
		return ""
	}
	values := url.Values{}
	for _, name := range tool.query {
		if v := stringArg(args, name); v != "" {
			values.Set(name, v)
		}
	}
	return values.Encode()
}

// stringArg reads an argument as a string, accepting the numbers a JSON decoder
// produces for integer fields such as limit and offset.
func stringArg(args map[string]interface{}, name string) string {
	switch v := args[name].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprintf("%d", int64(v))
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return ""
	}
}

// truncateAuditContent shortens the conversation text in an audit listing.
func truncateAuditContent(raw []byte) []byte {
	var payload struct {
		Events []map[string]interface{} `json:"events"`
		Total  int                      `json:"total"`
		Limit  int                      `json:"limit"`
		Offset int                      `json:"offset"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	for _, ev := range payload.Events {
		for _, field := range []string{"prompt", "response"} {
			if s, ok := ev[field].(string); ok {
				ev[field] = truncateRunes(s, auditContentLimit)
			}
		}
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return out
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…[truncated, see the web admin for the full text]"
}

// prettyJSON re-indents a JSON document for the model, matching what list_agents
// already returns. Non-JSON input is returned as-is.
func prettyJSON(raw []byte) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

// loopbackRecorder captures the in-process admin response.
type loopbackRecorder struct {
	headers http.Header
	status  int
	body    bytes.Buffer
	wrote   bool
}

func (r *loopbackRecorder) Header() http.Header { return r.headers }

func (r *loopbackRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }

func (r *loopbackRecorder) WriteHeader(status int) {
	if !r.wrote {
		r.status = status
		r.wrote = true
	}
}

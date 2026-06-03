import type {
  Agent,
  AgentListResponse,
  AdminAgent,
  AdminGroup,
  AdminLLMModel,
  AdminMCPServer,
  AdminUser,
  BasicAuthUser,
  ChartData,
  ErrorEvent,
  ErrorStat,
  GlobalStats,
  LLMModelOption,
  MCPOAuth2ScopeMapping,
  MCPServer,
  MCPTool,
  MultiAgentGroup,
  PaginatedSessionResponse,
  ResourceRanking,
  ResourceStats,
  Session,
  SessionListResponse,
  SlackIntegration,
  SlackTestResponse,
  SlackUserLink,
  TimelineBucket,
  User,
  UserDetailStats,
  UserStat,
} from "./types";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function fetchApi<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const url = `${API_BASE_URL}${endpoint}`;

  const response = await fetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
  });

  if (response.status === 401) {
    if (typeof window !== "undefined" && window.location.pathname !== "/login") {
      window.location.href = "/login";
    }
    throw new ApiError(401, "Session expired");
  }

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      errorData.error || `HTTP error ${response.status}`
    );
  }

  if (response.status === 204 || response.headers.get("content-length") === "0") {
    return undefined as T;
  }
  return response.json();
}

// User API
export async function getMe(): Promise<User> {
  return fetchApi<User>("/api/me");
}

// Agents API
export async function getAgents(): Promise<Agent[]> {
  const data = await fetchApi<AgentListResponse>("/api/agents");
  return data.agents;
}

// Sessions API (proxied to agents)
export async function getSessions(agentId: string): Promise<Session[]> {
  const data = await fetchApi<SessionListResponse>(
    `/api/agents/${agentId}/sessions`,
    { cache: "no-store" }
  );
  return data.sessions || [];
}

export async function getSession(
  agentId: string,
  sessionId: string
): Promise<Session> {
  return fetchApi<Session>(`/api/agents/${agentId}/sessions/${sessionId}`, {
    cache: "no-store",
  });
}

export async function getSessionPaginated(
  agentId: string,
  sessionId: string,
  limit: number,
  before?: number
): Promise<PaginatedSessionResponse> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (before !== undefined) {
    params.set("before", String(before));
  }
  return fetchApi<PaginatedSessionResponse>(
    `/api/agents/${agentId}/sessions/${sessionId}?${params.toString()}`,
    { cache: "no-store" }
  );
}

export async function renameSession(
  agentId: string,
  sessionId: string,
  newName: string
): Promise<Session> {
  return fetchApi<Session>(`/api/agents/${agentId}/sessions/${sessionId}`, {
    method: "PATCH",
    body: JSON.stringify({ session_name: newName }),
  });
}

export async function deleteSession(
  agentId: string,
  sessionId: string
): Promise<void> {
  await fetchApi(`/api/agents/${agentId}/sessions/${sessionId}`, {
    method: "DELETE",
  });
}

// Persist extracted charts to an assistant message in a session.
// assistantOffset: 0 = last assistant, 1 = second-to-last, etc.
export async function patchSessionCharts(
  agentId: string,
  sessionId: string,
  charts: ChartData[],
  assistantOffset = 0
): Promise<void> {
  await fetchApi(`/api/agents/${agentId}/sessions/${sessionId}/charts`, {
    method: "POST",
    body: JSON.stringify({ charts, assistant_offset: assistantOffset }),
  });
}

// Get the chat endpoint URL
export function getChatEndpoint(agentId: string): string {
  return `${API_BASE_URL}/api/agents/${agentId}/chat`;
}

// Auth Providers API
export interface AuthProvider {
  name: string;
  type: "oidc" | "oauth" | "basic";
  login_url: string;
}

export async function getAuthProviders(): Promise<AuthProvider[]> {
  const url = `${API_BASE_URL}/auth/providers`;
  const response = await fetch(url);
  if (!response.ok) return [];
  return response.json();
}

// Auth API
export interface AuthSessionResponse {
  authenticated: boolean;
  email?: string;
  name?: string;
  groups?: string[];
  github_connected?: boolean;
  github_username?: string;
}

export async function getAuthSession(): Promise<AuthSessionResponse> {
  const response = await fetch(`${API_BASE_URL}/auth/session`, {
    credentials: "include",
  });
  return response.json();
}

export async function logout(): Promise<{ ok: boolean; logout_url?: string }> {
  const response = await fetch(`${API_BASE_URL}/auth/logout`, {
    method: "POST",
    credentials: "include",
  });
  return response.json();
}

// GitHub OAuth API
export async function disconnectGitHub(): Promise<{ ok: boolean }> {
  const response = await fetch(`${API_BASE_URL}/auth/github/disconnect`, {
    method: "POST",
    credentials: "include",
  });
  return response.json();
}

// MCP API
export async function getMCPServers(): Promise<MCPServer[]> {
  const data = await fetchApi<{ servers: MCPServer[] }>("/api/mcp/servers");
  return data.servers || [];
}

export async function getMCPTools(serverId: string): Promise<MCPTool[]> {
  const data = await fetchApi<{ tools: MCPTool[] }>(`/api/mcp/servers/${serverId}/tools`);
  return data.tools || [];
}

export async function reconnectMCPServer(serverId: string): Promise<MCPServer> {
  return fetchApi<MCPServer>(`/api/mcp/servers/${serverId}/reconnect`, {
    method: "POST",
  });
}

export function getMCPChatEndpoint(serverId: string): string {
  return `${API_BASE_URL}/api/mcp/servers/${serverId}/chat`;
}

export function getMultiMCPChatEndpoint(): string {
  return `${API_BASE_URL}/api/mcp/chat`;
}

// MCP OAuth2 API
export interface MCPOAuth2Status {
  auth_type: string;
  connected: boolean;
  scopes?: string;
}

export async function getMCPOAuth2Status(serverId: string): Promise<MCPOAuth2Status> {
  return fetchApi<MCPOAuth2Status>(`/api/mcp/servers/${serverId}/oauth2/status`);
}

export async function disconnectMCPOAuth2(serverId: string): Promise<{ ok: boolean }> {
  return fetchApi<{ ok: boolean }>(`/api/mcp/servers/${serverId}/oauth2/disconnect`, {
    method: "POST",
  });
}

export function getMCPOAuth2LoginURL(serverId: string, returnUrl?: string): string {
  const params = returnUrl ? `?return_url=${encodeURIComponent(returnUrl)}` : "";
  return `${API_BASE_URL}/auth/mcp-oauth/${serverId}/login${params}`;
}

// Admin MCP OAuth2 Scope Mappings
export async function getMCPScopeMappings(serverId: string): Promise<MCPOAuth2ScopeMapping[]> {
  const data = await fetchApi<{ mappings: MCPOAuth2ScopeMapping[] }>(`/api/admin/mcp/${serverId}/scope-mappings`);
  return data.mappings || [];
}

export async function upsertMCPScopeMapping(serverId: string, groupName: string, scopes: string): Promise<void> {
  await fetchApi(`/api/admin/mcp/${serverId}/scope-mappings`, {
    method: "PUT",
    body: JSON.stringify({ group_name: groupName, scopes }),
  });
}

export async function deleteMCPScopeMapping(serverId: string, mappingId: string): Promise<void> {
  await fetchApi(`/api/admin/mcp/${serverId}/scope-mappings/${mappingId}`, {
    method: "DELETE",
  });
}

// MCP Sessions
export async function getMCPSessions(serverId: string): Promise<Session[]> {
  const data = await fetchApi<SessionListResponse>(`/api/mcp/servers/${serverId}/sessions`);
  return data.sessions || [];
}

export async function getMCPSession(serverId: string, sessionId: string): Promise<Session> {
  return fetchApi<Session>(`/api/mcp/servers/${serverId}/sessions/${sessionId}`);
}

export async function renameMCPSession(serverId: string, sessionId: string, newName: string): Promise<Session> {
  return fetchApi<Session>(`/api/mcp/servers/${serverId}/sessions/${sessionId}`, {
    method: "PATCH",
    body: JSON.stringify({ session_name: newName }),
  });
}

export async function deleteMCPSession(serverId: string, sessionId: string): Promise<void> {
  await fetchApi(`/api/mcp/servers/${serverId}/sessions/${sessionId}`, {
    method: "DELETE",
  });
}

// Multi-MCP Sessions
export async function getMultiMCPSessions(): Promise<Session[]> {
  const data = await fetchApi<SessionListResponse>("/api/mcp/sessions");
  return data.sessions || [];
}

export async function getMultiMCPSession(sessionId: string): Promise<Session> {
  return fetchApi<Session>(`/api/mcp/sessions/${sessionId}`);
}

export async function renameMultiMCPSession(sessionId: string, newName: string): Promise<Session> {
  return fetchApi<Session>(`/api/mcp/sessions/${sessionId}`, {
    method: "PATCH",
    body: JSON.stringify({ session_name: newName }),
  });
}

export async function deleteMultiMCPSession(sessionId: string): Promise<void> {
  await fetchApi(`/api/mcp/sessions/${sessionId}`, {
    method: "DELETE",
  });
}

// Admin API
export async function getAdminAgents(): Promise<AdminAgent[]> {
  const data = await fetchApi<{ agents: AdminAgent[] }>("/api/admin/agents");
  return data.agents || [];
}

export async function createAdminAgent(agent: Partial<AdminAgent>): Promise<AdminAgent> {
  return fetchApi<AdminAgent>("/api/admin/agents", {
    method: "POST",
    body: JSON.stringify(agent),
  });
}

export async function updateAdminAgent(id: string, agent: Partial<AdminAgent>): Promise<AdminAgent> {
  return fetchApi<AdminAgent>(`/api/admin/agents/${id}`, {
    method: "PUT",
    body: JSON.stringify(agent),
  });
}

export async function deleteAdminAgent(id: string): Promise<void> {
  await fetchApi(`/api/admin/agents/${id}`, { method: "DELETE" });
}

export async function updateAdminAgentPermissions(id: string, allowedUsers: string[], allowedGroups: string[]): Promise<void> {
  await fetchApi(`/api/admin/agents/${id}/permissions`, {
    method: "PUT",
    body: JSON.stringify({ allowed_users: allowedUsers, allowed_groups: allowedGroups }),
  });
}

export async function getAdminMCPServers(): Promise<AdminMCPServer[]> {
  const data = await fetchApi<{ servers: AdminMCPServer[] }>("/api/admin/mcp");
  return data.servers || [];
}

export async function createAdminMCPServer(server: Partial<AdminMCPServer>): Promise<AdminMCPServer> {
  return fetchApi<AdminMCPServer>("/api/admin/mcp", {
    method: "POST",
    body: JSON.stringify(server),
  });
}

export async function updateAdminMCPServer(id: string, server: Partial<AdminMCPServer>): Promise<AdminMCPServer> {
  return fetchApi<AdminMCPServer>(`/api/admin/mcp/${id}`, {
    method: "PUT",
    body: JSON.stringify(server),
  });
}

export async function deleteAdminMCPServer(id: string): Promise<void> {
  await fetchApi(`/api/admin/mcp/${id}`, { method: "DELETE" });
}

export async function getAdminUsers(): Promise<AdminUser[]> {
  const data = await fetchApi<{ users: AdminUser[] }>("/api/admin/users");
  return data.users || [];
}

export async function updateAdminUserRole(email: string, role: string): Promise<void> {
  await fetchApi(`/api/admin/users/${encodeURIComponent(email)}/role`, {
    method: "PUT",
    body: JSON.stringify({ role }),
  });
}

// Basic Auth Users API
export async function getBasicAuthUsers(): Promise<BasicAuthUser[]> {
  const data = await fetchApi<{ users: BasicAuthUser[] }>("/api/admin/basic-auth/users");
  return data.users || [];
}

export async function createBasicAuthUser(user: { username: string; email: string; password: string }): Promise<void> {
  await fetchApi("/api/admin/basic-auth/users", {
    method: "POST",
    body: JSON.stringify(user),
  });
}

export async function deleteBasicAuthUser(id: string): Promise<void> {
  await fetchApi(`/api/admin/basic-auth/users/${id}`, {
    method: "DELETE",
  });
}

// User-facing Groups API
interface GroupApiResponse {
  id: string;
  name: string;
  agentIds: string[];
  allowed_users?: string[];
  allowed_groups?: string[];
  created_at: string;
}

function mapGroupResponse(g: GroupApiResponse): MultiAgentGroup {
  return {
    id: g.id,
    name: g.name,
    agentIds: g.agentIds,
    allowedUsers: g.allowed_users,
    allowedGroups: g.allowed_groups,
    createdAt: new Date(g.created_at).getTime(),
  };
}

export async function getGroups(): Promise<MultiAgentGroup[]> {
  const data = await fetchApi<{ groups: GroupApiResponse[] }>("/api/groups");
  return (data.groups || []).map(mapGroupResponse);
}

export async function createGroup(name: string, agentIds: string[], allowedUsers?: string[], allowedGroups?: string[]): Promise<MultiAgentGroup> {
  const body: Record<string, unknown> = { name, agentIds };
  if (allowedUsers && allowedUsers.length > 0) {
    body.allowed_users = allowedUsers;
  }
  if (allowedGroups && allowedGroups.length > 0) {
    body.allowed_groups = allowedGroups;
  }
  const data = await fetchApi<GroupApiResponse>("/api/groups", {
    method: "POST",
    body: JSON.stringify(body),
  });
  return mapGroupResponse(data);
}

export async function updateGroup(groupId: string, updates: { name?: string; agentIds?: string[]; allowed_users?: string[]; allowed_groups?: string[] }): Promise<MultiAgentGroup> {
  const data = await fetchApi<GroupApiResponse>(`/api/groups/${groupId}`, {
    method: "PUT",
    body: JSON.stringify(updates),
  });
  return mapGroupResponse(data);
}

export async function deleteGroup(groupId: string): Promise<void> {
  await fetchApi(`/api/groups/${groupId}`, { method: "DELETE" });
}

// Group Sessions API
export async function getGroupSessions(groupId: string): Promise<Session[]> {
  const data = await fetchApi<SessionListResponse>(`/api/groups/${groupId}/sessions`, { cache: "no-store" });
  return data.sessions || [];
}

export async function addGroupSession(groupId: string, sessionId: string): Promise<void> {
  await fetchApi(`/api/groups/${groupId}/sessions`, {
    method: "POST",
    body: JSON.stringify({ session_id: sessionId }),
  });
}

export async function removeGroupSession(groupId: string, sessionId: string): Promise<void> {
  await fetchApi(`/api/groups/${groupId}/sessions/${sessionId}`, { method: "DELETE" });
}

// Read State API (unread tracking)
export async function getReadState(): Promise<Record<string, number>> {
  return fetchApi<Record<string, number>>("/api/read-state");
}

export async function markSessionRead(sessionId: string, count: number): Promise<void> {
  await fetchApi(`/api/read-state/${sessionId}`, {
    method: "PUT",
    body: JSON.stringify({ count }),
  });
}

export async function migrateReadState(state: Record<string, number>): Promise<void> {
  await fetchApi("/api/read-state", {
    method: "PUT",
    body: JSON.stringify(state),
  });
}

// Session Sharing API
export interface ShareResponse {
  token: string;
  url: string;
  expires_at: string;
}

export interface SharedSessionInfo {
  token: string;
  agent_id: string;
  agent_name: string;
  session_name: string;
  shared_by: string;
  message_count: number;
  expires_at: string;
}

export async function shareSession(
  agentId: string,
  sessionId: string,
  expiresInHours?: number
): Promise<ShareResponse> {
  return fetchApi<ShareResponse>(`/api/agents/${agentId}/sessions/${sessionId}/share`, {
    method: "POST",
    body: JSON.stringify(expiresInHours ? { expires_in_hours: expiresInHours } : {}),
  });
}

export async function revokeShare(
  agentId: string,
  sessionId: string
): Promise<void> {
  await fetchApi(`/api/agents/${agentId}/sessions/${sessionId}/share`, {
    method: "DELETE",
  });
}

export async function getSharedSession(token: string): Promise<SharedSessionInfo> {
  return fetchApi<SharedSessionInfo>(`/api/shared/${token}`);
}

export async function cloneSharedSession(token: string): Promise<{ session: Session }> {
  return fetchApi<{ session: Session }>(`/api/shared/${token}/clone`, {
    method: "POST",
  });
}

// Subscribe to real-time session events
export function getSessionSubscribeUrl(sessionId: string): string {
  return `${API_BASE_URL}/api/sessions/${sessionId}/subscribe`;
}

// Subscribe to real-time read state events
export function getReadStateSubscribeUrl(): string {
  return `${API_BASE_URL}/api/read-state/subscribe`;
}

// Admin Groups API
export async function getAdminGroups(): Promise<AdminGroup[]> {
  const data = await fetchApi<{ groups: AdminGroup[] }>("/api/admin/groups");
  return data.groups || [];
}

export async function createAdminGroup(group: Partial<AdminGroup>): Promise<AdminGroup> {
  return fetchApi<AdminGroup>("/api/admin/groups", {
    method: "POST",
    body: JSON.stringify(group),
  });
}

export async function updateAdminGroup(id: string, group: Partial<AdminGroup>): Promise<AdminGroup> {
  return fetchApi<AdminGroup>(`/api/admin/groups/${id}`, {
    method: "PUT",
    body: JSON.stringify(group),
  });
}

export async function deleteAdminGroup(id: string): Promise<void> {
  await fetchApi(`/api/admin/groups/${id}`, { method: "DELETE" });
}

export async function updateAdminGroupPermissions(id: string, allowedUsers: string[], allowedGroups: string[]): Promise<void> {
  await fetchApi(`/api/admin/groups/${id}/permissions`, {
    method: "PUT",
    body: JSON.stringify({ allowed_users: allowedUsers, allowed_groups: allowedGroups }),
  });
}

// Admin LLM API
export async function getAdminLLMs(): Promise<AdminLLMModel[]> {
  const data = await fetchApi<{ models: AdminLLMModel[] }>("/api/admin/llm");
  return data.models || [];
}

export async function createAdminLLM(model: Partial<AdminLLMModel>): Promise<AdminLLMModel> {
  return fetchApi<AdminLLMModel>("/api/admin/llm", {
    method: "POST",
    body: JSON.stringify(model),
  });
}

export async function updateAdminLLM(id: string, model: Partial<AdminLLMModel>): Promise<AdminLLMModel> {
  return fetchApi<AdminLLMModel>(`/api/admin/llm/${id}`, {
    method: "PUT",
    body: JSON.stringify(model),
  });
}

export async function deleteAdminLLM(id: string): Promise<void> {
  await fetchApi(`/api/admin/llm/${id}`, { method: "DELETE" });
}

// Available LLM models (from /api/config)
export async function getAvailableModels(): Promise<LLMModelOption[]> {
  const data = await fetchApi<{ available_models: LLMModelOption[]; features: Record<string, boolean> }>("/api/config");
  return data.available_models || [];
}

// --- Admin Metrics (Observability) ---

function buildMetricsParams(from?: string, to?: string, interval?: string, limit?: number): string {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  if (interval) params.set("interval", interval);
  if (limit) params.set("limit", String(limit));
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

export async function getMetricsOverview(from?: string, to?: string): Promise<GlobalStats> {
  return fetchApi<GlobalStats>(`/api/admin/metrics/overview${buildMetricsParams(from, to)}`);
}

export async function getMetricsTimeline(from?: string, to?: string, interval?: string): Promise<TimelineBucket[]> {
  return fetchApi<TimelineBucket[]>(`/api/admin/metrics/overview/timeline${buildMetricsParams(from, to, interval)}`);
}

export async function getMetricsTopResources(from?: string, to?: string, limit?: number): Promise<ResourceRanking[]> {
  return fetchApi<ResourceRanking[]>(`/api/admin/metrics/overview/top${buildMetricsParams(from, to, undefined, limit)}`);
}

export async function getMetricsOverviewUsers(from?: string, to?: string, limit?: number): Promise<UserStat[]> {
  return fetchApi<UserStat[]>(`/api/admin/metrics/overview/users${buildMetricsParams(from, to, undefined, limit)}`);
}

export type MetricsResourceType = "agents" | "mcp";

export async function getResourceMetrics(type: MetricsResourceType, id: string, from?: string, to?: string): Promise<ResourceStats> {
  return fetchApi<ResourceStats>(`/api/admin/metrics/${type}/${id}${buildMetricsParams(from, to)}`);
}

export async function getResourceTimeline(type: MetricsResourceType, id: string, from?: string, to?: string, interval?: string): Promise<TimelineBucket[]> {
  return fetchApi<TimelineBucket[]>(`/api/admin/metrics/${type}/${id}/timeline${buildMetricsParams(from, to, interval)}`);
}

export async function getResourceUsers(type: MetricsResourceType, id: string, from?: string, to?: string): Promise<UserStat[]> {
  return fetchApi<UserStat[]>(`/api/admin/metrics/${type}/${id}/users${buildMetricsParams(from, to)}`);
}

export async function getResourceErrors(type: MetricsResourceType, id: string, from?: string, to?: string): Promise<ErrorStat[]> {
  return fetchApi<ErrorStat[]>(`/api/admin/metrics/${type}/${id}/errors${buildMetricsParams(from, to)}`);
}

export async function getResourceErrorEvents(type: MetricsResourceType, id: string, from?: string, to?: string, limit?: number): Promise<ErrorEvent[]> {
  return fetchApi<ErrorEvent[]>(`/api/admin/metrics/${type}/${id}/error-events${buildMetricsParams(from, to, undefined, limit)}`);
}

export async function getMetricsOverviewErrors(from?: string, to?: string): Promise<ErrorStat[]> {
  return fetchApi<ErrorStat[]>(`/api/admin/metrics/overview/errors${buildMetricsParams(from, to)}`);
}

export async function getMetricsOverviewErrorEvents(from?: string, to?: string, limit?: number): Promise<ErrorEvent[]> {
  return fetchApi<ErrorEvent[]>(`/api/admin/metrics/overview/error-events${buildMetricsParams(from, to, undefined, limit)}`);
}

// --- User Metrics ---

function buildUserMetricsParams(from?: string, to?: string, interval?: string, limit?: number, resourceType?: string, resourceId?: string): string {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  if (interval) params.set("interval", interval);
  if (limit) params.set("limit", String(limit));
  if (resourceType) params.set("resource_type", resourceType);
  if (resourceId) params.set("resource_id", resourceId);
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

export async function getUserMetrics(email: string, from?: string, to?: string, resourceType?: string, resourceId?: string): Promise<UserDetailStats> {
  return fetchApi<UserDetailStats>(`/api/admin/metrics/users/${encodeURIComponent(email)}${buildUserMetricsParams(from, to, undefined, undefined, resourceType, resourceId)}`);
}

export async function getUserTimeline(email: string, from?: string, to?: string, interval?: string, resourceType?: string, resourceId?: string): Promise<TimelineBucket[]> {
  return fetchApi<TimelineBucket[]>(`/api/admin/metrics/users/${encodeURIComponent(email)}/timeline${buildUserMetricsParams(from, to, interval, undefined, resourceType, resourceId)}`);
}

export async function getUserTopResources(email: string, from?: string, to?: string, limit?: number): Promise<ResourceRanking[]> {
  return fetchApi<ResourceRanking[]>(`/api/admin/metrics/users/${encodeURIComponent(email)}/resources${buildUserMetricsParams(from, to, undefined, limit)}`);
}

// Slack Integration API
export async function getAgentSlack(agentId: string): Promise<SlackIntegration> {
  return fetchApi<SlackIntegration>(`/api/admin/agents/${agentId}/slack`);
}

export async function upsertAgentSlack(
  agentId: string,
  data: { bot_token?: string; app_token?: string; enabled: boolean }
): Promise<SlackIntegration> {
  return fetchApi<SlackIntegration>(`/api/admin/agents/${agentId}/slack`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function deleteAgentSlack(agentId: string): Promise<void> {
  return fetchApi<void>(`/api/admin/agents/${agentId}/slack`, {
    method: "DELETE",
  });
}

export async function testAgentSlack(
  botToken: string,
  appToken: string,
  agentId: string
): Promise<SlackTestResponse> {
  return fetchApi<SlackTestResponse>(`/api/admin/agents/${agentId}/slack/test`, {
    method: "POST",
    body: JSON.stringify({ bot_token: botToken, app_token: appToken }),
  });
}

// Slack Sessions API
export async function getSlackSessions(): Promise<Session[]> {
  const data = await fetchApi<SessionListResponse>("/api/slack/sessions", { cache: "no-store" });
  return data.sessions || [];
}

// Slack User Links API (admin)
export async function getSlackUserLinks(): Promise<SlackUserLink[]> {
  return fetchApi<SlackUserLink[]>("/api/admin/slack/links");
}

export async function revokeSlackUserLink(slackUserId: string): Promise<void> {
  return fetchApi<void>(`/api/admin/slack/links?slack_user_id=${encodeURIComponent(slackUserId)}`, {
    method: "DELETE",
  });
}

export async function revokeSlackUserGitHub(slackUserId: string): Promise<void> {
  return fetchApi<void>(`/api/admin/slack/links/github?slack_user_id=${encodeURIComponent(slackUserId)}`, {
    method: "DELETE",
  });
}

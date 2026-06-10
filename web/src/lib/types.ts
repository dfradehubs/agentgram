// Agent types
export interface Agent {
  id: string;
  name: string;
  description: string;
  category: string;
  protocol: string;
  status: string;
  require_github_token?: boolean;
}

export interface AgentListResponse {
  agents: Agent[];
}

// Session types (managed by agents)
export interface Session {
  session_id: string;
  session_name: string;
  user_id: string;
  app_name: string;
  agent_ids?: string[];
  is_multi_agent: boolean;
  group_id?: string;
  source?: string;             // "slack" or undefined (web)
  slack_thread_id?: string;    // For syncing sibling sessions
  created_at: number;
  last_activity: number;
  message_count: number;
  messages?: Message[];
  active_run?: boolean;        // True when a run is in flight (for live reconnect)
}

export interface SessionListResponse {
  sessions: Session[];
}

// Paginated session response from the API
export interface PaginatedSessionResponse {
  session: Session;
  messages: Message[];
  has_more: boolean;
  next_cursor?: number;
}

// Attachment types
export interface Attachment {
  filename: string;
  content_type: string;
  data: string; // base64
}

// Tool call/result as stored in session history (from agent API)
interface StoredToolCall {
  id?: string;
  name: string;
  args: Record<string, unknown>;
}

interface StoredToolResult {
  id?: string;
  name: string;
  response: Record<string, unknown>;
}

// Chart visualization types
export interface ChartDataset {
  label: string;
  data: number[];
  color?: string;
}

export interface ChartData {
  chartType: "bar" | "line" | "pie" | "area";
  title?: string;
  description?: string;
  xAxisLabel?: string;
  yAxisLabel?: string;
  labels: string[];
  datasets: ChartDataset[];
  options?: {
    stacked?: boolean;
    horizontal?: boolean;
    showLegend?: boolean;
  };
}

// ContentPart represents an ordered segment of an assistant message (text, tool reference, or chart)
interface ContentPart {
  type: "text" | "tool_use" | "chart";
  text?: string;        // For type="text"
  tool_index?: number;  // For type="tool_use", index into tool_calls
  chart?: ChartData;    // For type="chart"
}

export interface Message {
  role: "user" | "assistant" | "system";
  content: string;
  agent_id?: string;
  user_name?: string; // Display name of the user who sent this message
  user_email?: string; // Email of the user who sent this message
  is_admin?: boolean; // Whether the user who sent this message is an admin
  isThinking?: boolean; // Intermediate reasoning step from multi-sequence runs
  broadcast_agent_ids?: string[]; // Which agents this user message was directed to (multi-agent groups)
  attachments?: Attachment[]; // File attachments
  tool_calls?: StoredToolCall[]; // Tool calls made by assistant (from session history)
  tool_results?: StoredToolResult[]; // Tool results (from session history)
  content_parts?: ContentPart[]; // Ordered text/tool interleaving for reconstruction
}

// Tool call types (used by both MCP and agent chat)
export interface ToolCall {
  toolCallId: string;
  toolName: string;
  args: string;
  result?: string;
  isComplete: boolean;
  serverId?: string;
}

export type TimelineItem =
  | { type: "message"; message: Message }
  | { type: "tool_group"; toolCalls: ToolCall[]; agentId?: string }
  | { type: "chart"; chart: ChartData; agentId?: string };

// Multi-agent group (persisted in database)
export interface MultiAgentGroup {
  id: string;
  name: string;
  agentIds: string[];
  allowedUsers?: string[];
  allowedGroups?: string[];
  createdAt: number;
}

// Admin group (full data for admin panel)
export interface AdminGroup {
  id: string;
  name: string;
  agent_ids: string[];
  created_by: string;
  allowed_users: string[];
  allowed_groups: string[];
  created_at: string;
  updated_at: string;
}

// Inherited permission (from agent group)
export interface InheritedPermission {
  value: string;
  from_group: string;
}

// User types
export interface User {
  email: string;
  name?: string;
  groups?: string[];
  isAdmin?: boolean;
  githubConnected?: boolean;
  githubUsername?: string;
}

// Admin types
export interface CustomFormatConfig {
  request_template?: string;
  response_content_path?: string;
  request_method?: string;
  request_content_type?: string;
}

export interface AgentApiKeyRule {
  subject_type: "user" | "group";
  subject: string;
  api_key: string;
}

export interface AdminAgent {
  id: string;
  name: string;
  description: string;
  category: string;
  protocol: string;
  endpoint: string;
  agent_card_path?: string;
  forward_authorization: boolean;
  auth_type?: string;
  bearer_token?: string;
  auth_header_name?: string;
  api_key_rules?: AgentApiKeyRule[];
  require_github_token: boolean;
  pipeline_final_agent?: string;
  adk_app_name?: string;
  adk_user_id?: string;
  max_context_tokens?: number;
  summarize_threshold?: number;
  headers: Record<string, string>;
  rate_limit?: { requests_per_minute: number; requests_per_hour: number };
  health_check?: { enabled: boolean; url: string; endpoint: string; interval_seconds: number; timeout_seconds: number };
  polling?: { interval_ms: number; timeout_seconds: number };
  custom_format?: CustomFormatConfig;
  allowed_users: string[];
  allowed_groups: string[];
  inherited_permissions?: {
    users: InheritedPermission[];
    groups: InheritedPermission[];
  };
  status: string;
}

export interface AdminMCPServer {
  id: string;
  name: string;
  description: string;
  transport: string;
  url: string;
  headers: Record<string, string>;
  forward_auth: boolean;
  allowed_users: string[];
  allowed_groups: string[];
  auth_type?: string;
  oauth2_auth_server_url?: string;
  oauth2_client_id?: string;
  oauth2_client_secret?: string;
  oauth2_scopes?: string;
  bearer_token?: string;
}

export interface MCPOAuth2ScopeMapping {
  id: string;
  mcp_server_id: string;
  group_name: string;
  scopes: string;
  created_at: string;
}

export interface AdminUser {
  id: string;
  email: string;
  role: string;
  protected: boolean;
  created_at: string;
  updated_at: string;
  last_access_at: string | null;
}

export interface BasicAuthUser {
  id: string;
  username: string;
  email: string;
  created_at: string;
  updated_at: string;
}

export interface AdminLLMModel {
  id: string;
  name: string;
  provider: string;
  model: string;
  api_key: string;
  role: string;
  enabled: boolean;
  is_default: boolean;
}

export interface LLMModelOption {
  id: string;
  name: string;
  provider: string;
  default?: boolean;
}

// MCP types
export interface MCPServer {
  id: string;
  name: string;
  description: string;
  transport: string;
  status: "connected" | "error" | "disconnected";
  status_error?: string;
  tool_count: number;
  auth_type?: string;
  oauth2_connected?: boolean;
}

export interface MCPTool {
  name: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
}

// Slack integration types
export interface SlackIntegration {
  agent_id: string;
  enabled: boolean;
  workspace_id: string;
  workspace_name: string;
  status: "connected" | "disconnected" | "error";
  status_message: string;
  has_bot_token: boolean;
  has_app_token: boolean;
  created_at: string;
  updated_at: string;
}

export interface SlackTestResponse {
  workspace_id: string;
  workspace_name: string;
}

export interface SlackUserLink {
  slack_user_id: string;
  email: string;
  has_github: boolean;
  created_at: string;
  updated_at: string;
}

// Metrics types (matches Go models in api/internal/models/chat_event.go)

export interface TokenUsage {
  input: number;
  output: number;
  total: number;
}

export interface GlobalStats {
  total_requests: number;
  success_count: number;
  error_count: number;
  error_rate: number;
  avg_duration_ms: number;
  p95_duration_ms: number;
  unique_users: number;
  active_agents: number;
  token_usage?: TokenUsage;
}

export interface ResourceStats {
  total_requests: number;
  success_count: number;
  error_count: number;
  error_rate: number;
  avg_duration_ms: number;
  p95_duration_ms: number;
  avg_ttfb_ms?: number;
  token_usage?: TokenUsage;
  unique_users: number;
  context_rotations: number;
  total_tool_calls: number;
  llm_model?: string;
}

export interface TimelineBucket {
  timestamp: string;
  requests: number;
  errors: number;
  avg_duration_ms: number;
  avg_ttfb_ms?: number;
}

export interface UserStat {
  user_email: string;
  requests: number;
  errors: number;
  last_access: string;
}

export interface ErrorStat {
  error_type: string;
  count: number;
  last_seen: string;
  last_msg?: string;
}

export interface UserDetailStats {
  total_requests: number;
  error_rate: number;
  avg_duration_ms: number;
  p95_duration_ms: number;
  token_usage?: TokenUsage;
  active_agents: number;
}

export interface ErrorEvent {
  id: string;
  timestamp: string;
  resource_type: string;
  resource_id: string;
  resource_name: string;
  error_type: string;
  error_msg: string;
  duration_ms: number;
  user_email: string;
}

export interface ResourceRanking {
  resource_type: string;
  resource_id: string;
  resource_name: string;
  requests: number;
  error_rate: number;
  avg_duration_ms: number;
}

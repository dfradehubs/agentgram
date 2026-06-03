import type { Attachment, Message, TimelineItem } from "@/lib/types";

export interface MCPConfig {
  serverIds: string[];
  modelId: string;
}

export interface UseChatOptions {
  agentId: string;
  agentName?: string;
  sessionName?: string;
  sessionId?: string;
  sessionResetKey?: number;
  initialMessages?: Message[];
  onSessionCreated?: (sessionId?: string, sessionName?: string) => void;
  chatEndpoint?: string; // Override default chat endpoint
  mcpConfig?: MCPConfig; // MCP mode: use MCP endpoint + include model_id/server_ids
  groupId?: string; // Group ID for collaborative sessions
  userName?: string; // Display name of current user (for multi-user group sessions)
}

export interface UseChatReturn {
  messages: Message[];
  timeline: TimelineItem[];
  input: string;
  setInput: (input: string) => void;
  sendMessage: (targetAgentId?: string, sendContext?: boolean, attachments?: Attachment[], textOverride?: string) => void;
  sendMultiple: (targetAgentIds: string[], sendContext?: boolean, attachments?: Attachment[], textOverride?: string, baseMessagesOverride?: Message[]) => void;
  activeStreamAgentIds: string[];
  isLoading: boolean;
  error: string | null;
  errorType: "github_auth" | "mcp_oauth2" | null;
  stop: () => void;
  retry: (targetMessage?: Message, targetAgentIds?: string[]) => void;
  replaceMessages: (newMessages: Message[]) => void;
  prependMessages: (olderMessages: Message[]) => void;
  isReconnecting: boolean;
}

/** Parameters for processing a single-agent SSE stream */
export interface SSEStreamParams {
  reader: ReadableStreamDefaultReader<Uint8Array>;
  gen: number;
  allMessages: Message[];
  baseTimeline: TimelineItem[];
  effectiveAgentId: string;
}

/** Parameters for processing a parallel multi-agent SSE stream */
export interface AgentStreamParams {
  reader: ReadableStreamDefaultReader<Uint8Array>;
  gen: number;
  targetAgentId: string;
  perAgentItems: Map<string, TimelineItem[]>;
  perAgentFinalMsgs: Map<string, Message>;
  respondedOrder: string[];
  rebuildTimeline: () => void;
  onRunStarted?: () => void;
}

/** Internal segment type for tracking thinking vs regular content */
export interface ContentSegment {
  content: string;
  isThinking: boolean;
}

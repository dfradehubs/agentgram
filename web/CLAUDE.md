# CLAUDE.md - Web

## Overview

Next.js web with custom chat UI that consumes AG-UI SSE events directly from the Go API.

## Tech Stack

- **Next.js 16** with App Router
- **Tailwind CSS v4** for styling
- **TypeScript**
- **Pino** for logging (JSON in production, pretty in dev)

## Architecture

```
Web ──(SSE/AG-UI)──> API ──> Agents
```

The web communicates directly with the API via `fetch` + `ReadableStream`. The API responds with Server-Sent Events (SSE) using the AG-UI event format.

### Key Components

- **AgentContext**: Manages available agents and current selection (30s polling)
- **SessionContext**: Manages chat sessions (single + multi-agent groups)
- **BackgroundStreamContext**: Manages SSE streams running in background when user navigates away
- **MCPContext**: Manages MCP server connections and sessions
- **useChat hook**: Unified SSE hook for agent chat, MCP chat, and multi-agent groups
- **Chat**: Unified chat component (agent 1:1, MCP, multi-agent group)
- **Sidebar**: Resizable agent list with expandable session lists

## Directory Structure

```
src/
├── app/
│   ├── layout.tsx              # Root layout with all providers
│   ├── page.tsx                # Main page (sidebar + chat)
│   ├── globals.css             # Theme variables
│   ├── admin/                  # Admin dashboard (agents, users, LLM, MCP)
│   ├── auth/[...path]/         # Auth routes (proxied to API)
│   ├── api/[...path]/          # API routes (proxied to API)
│   └── login/                  # Login page
├── components/
│   ├── layout/                 # Header, Sidebar, ChatArea
│   ├── agents/                 # AgentList, AgentItem
│   ├── sessions/               # SessionList, SessionItem, GroupItem, NewSessionButton, CreateMultiAgentDialog, EditGroupDialog
│   ├── chat/                   # Chat, AgentSelector, MarkdownMessage, ToolCallBlock, ThinkingBubbles, EmptyState, AttachmentPreview
│   ├── mcp/                    # MCPToolsPanel, MCPSessionList, MultiMCPItem, CreateMultiMCPDialog
│   ├── admin/                  # AgentForm, LLMForm, MCPForm, TagInput, AdminNav, observability/
│   ├── icons/                  # AgentgramLogo
│   └── ui/                     # shadcn/ui primitives (badge, button, dialog, etc.)
├── contexts/
│   ├── AgentContext.tsx         # Agent selection + polling
│   ├── SessionContext.tsx       # Single/multi-agent session management
│   ├── BackgroundStreamContext.tsx # Background SSE stream buffer + notifications
│   ├── MCPContext.tsx           # MCP server + session management
│   ├── ConfigContext.tsx        # App config (features, LLM models)
│   ├── PreferencesContext.tsx   # User theme/locale preferences
│   ├── ReadStateContext.tsx     # Read/unread state tracking for sessions
│   └── UserContext.tsx          # User auth state
├── hooks/
│   ├── useChat.ts              # Unified SSE chat hook (agent, MCP, multi-agent)
│   ├── useAgents.ts            # AgentContext wrapper
│   ├── useSessions.ts          # SessionContext wrapper
│   ├── usePreferences.ts       # localStorage preferences
│   ├── useReadState.ts         # Read/unread state hook
│   ├── useSessionSubscription.ts # Real-time session update subscription
│   └── useUser.ts              # UserContext wrapper
├── lib/
│   ├── api.ts                  # Fetch API client
│   ├── types.ts                # Shared TypeScript types
│   ├── logger.ts               # Pino logger configuration
│   ├── utils.ts                # Tailwind cn() utility
│   ├── agent-colors.ts         # Hash-based color assignment for agents
│   ├── notifications.ts        # Browser Notification API wrapper
│   ├── export-pdf.ts           # PDF export using pdfmake
│   ├── markdown-to-pdfmake.ts  # Markdown → pdfmake converter
│   ├── telemetry.ts            # Client-side metrics reporting
│   └── i18n/                   # Internationalization (es/en)
│       ├── index.ts            # useT() hook + getT() helper
│       └── translations.ts     # Translation strings
├── types/
│   └── pdfmake.d.ts            # Type declarations for pdfmake
└── middleware.ts                # Next.js middleware (local dev proxy)
```

## Commands

```bash
# Development
npm run dev           # Start dev server on :3000

# Build
npm run build         # Production build
npm run start         # Start production server

# Lint
npm run lint
```

## Configuration

Environment variables (`.env.local`):

```env
NEXT_PUBLIC_API_URL=http://localhost:8080  # API URL
LOG_LEVEL=info                             # Pino log level
API_URL_DEV=                               # Local dev: proxy /api/* and /auth/* to this URL
```

## SSE Chat Integration

### AG-UI Protocol (consumed directly)

The `useChat` hook parses AG-UI SSE events from the API:
- `RUN_STARTED` / `RUN_FINISHED` - Run lifecycle
- `TEXT_MESSAGE_START` - Signals assistant message begins (with optional `agentId` and `isThinking`)
- `TEXT_MESSAGE_CONTENT` - Streaming text delta (`event.delta`)
- `TEXT_MESSAGE_END` - Signals message complete
- `TOOL_CALL_START` / `TOOL_CALL_ARGS` / `TOOL_CALL_END` - Tool call lifecycle
- `RUN_ERROR` - Error during run

### Chat Modes

- **Single agent**: `POST /api/agents/{id}/chat` — standard chat with one agent
- **Multi-agent group**: Same endpoint but with `send_context: true` to propagate context between agents (delta-based via `PrepareMessagesForMultiAgent`)
- **MCP**: `POST /api/mcp/{serverId}/chat` or `POST /api/mcp/multi/chat` — chat with MCP server tools

### Unified Chat Component

`Chat.tsx` handles all modes (agent 1:1, MCP, multi-agent group) by detecting mode from context:
- **Agent mode**: Shows agent header with protocol badge, status, PDF export
- **MCP mode**: Shows server header with model selector, tools panel, reconnect
- **Multi-agent mode**: Shows group header with agent avatars, `AgentSelector` pills in input area to choose target agent

### Background Streams

When user navigates away from an active chat, the SSE stream transfers to `BackgroundStreamContext` which:
1. Buffers raw SSE events
2. Detects completion (`RUN_FINISHED`) → shows toast + browser notification
3. On return, creates a composite stream (buffered + live) for seamless resume

### Session Management

Sessions are managed by the API (stored in Redis):
1. Web fetches sessions via API: `GET /api/agents/{id}/sessions`
2. On first message in new chat, agent creates session
3. Session ID is passed as `session_id` in chat requests

## Security

Security headers are configured in `next.config.ts`:
- X-Content-Type-Options, X-Frame-Options, HSTS, Referrer-Policy, Permissions-Policy
- Applied to all routes via `headers()` config

## Theming

Dark mode is automatic via `prefers-color-scheme: dark`. Colors use Tailwind's zinc palette.

## Known Tech Debt

- **Missing memoization**: `MarkdownMessage`, `ToolCallBlock`, `SessionItem`, `AgentItem` should use `React.memo`.

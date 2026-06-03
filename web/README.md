# Agentgram Frontend

Next.js frontend with SSE streaming and the AG-UI protocol, providing a unified chat interface with multiple AI agents.

## Features

- Agent selector with permission-based filtering
- Separate chat sessions per agent
- Persistent conversation history
- Streaming support via the AG-UI protocol
- Automatic light/dark theme
- Responsive UI

## Tech Stack

- **Next.js 16** - React framework with App Router
- **AG-UI Protocol** - Direct SSE streaming with the AG-UI protocol
- **Tailwind CSS 4** - Utility-first styling
- **TypeScript** - Static typing

## Quick Start

```bash
# Install dependencies
npm install

# Start in development mode
npm run dev

# Build for production
npm run build
npm run start
```

The frontend will be available at http://localhost:3000

## Configuration

Create a `.env.local` file:

```env
# Backend URL
NEXT_PUBLIC_API_URL=http://localhost:8080
```

## Project Structure

```
src/
├── app/
│   ├── layout.tsx          # Root layout with providers
│   ├── page.tsx            # Main page
│   └── globals.css         # Global styles + theme variables
├── components/
│   ├── layout/
│   │   ├── Sidebar.tsx     # Sidebar with agents
│   │   ├── Header.tsx      # Header with status
│   │   └── ChatArea.tsx    # Main chat area
│   ├── agents/
│   │   ├── AgentList.tsx   # List of available agents
│   │   └── AgentItem.tsx   # Expandable agent item
│   ├── sessions/
│   │   ├── SessionList.tsx # List of sessions
│   │   ├── SessionItem.tsx # Session item (rename/delete)
│   │   └── NewSessionButton.tsx
│   ├── chat/
│   │   ├── AgentChat.tsx   # Chat with AG-UI streaming
│   │   └── EmptyState.tsx  # State when no agent is selected
├── contexts/
│   ├── AgentContext.tsx    # Global agent state
│   └── SessionContext.tsx  # Per-agent session state
├── hooks/
│   ├── useAgents.ts        # Hook for agents
│   └── useSessions.ts      # Hook for sessions
└── lib/
    ├── api.ts              # Backend API client
    └── types.ts            # TypeScript types
```

## Main Components

### AgentContext

Manages the list of available agents and the currently selected agent.

```tsx
const { agents, currentAgent, selectAgent } = useAgents();
```

### SessionContext

Manages the sessions of the current agent (loaded from the agent via the backend).

```tsx
const { sessions, currentSession, selectSession, createNewSession } = useSessions();
```


## Backend API

The frontend communicates with the backend through:

### Agents

```typescript
GET /api/agents              // List agents filtered by permissions
GET /api/agents/{id}         // Details of an agent
```

### Sessions (proxied to the agent)

```typescript
GET    /api/agents/{id}/sessions              // List sessions
GET    /api/agents/{id}/sessions/{sessionId}  // Get session with messages
PATCH  /api/agents/{id}/sessions/{sessionId}  // Rename
DELETE /api/agents/{id}/sessions/{sessionId}  // Delete
```

### Chat

```typescript
POST /api/agents/{id}/chat   // Send message (SSE response)
```

## Theme Customization

Colors are configured in `globals.css` using standard CSS variables and Tailwind CSS:

```css
:root {
  --primary-color: #3b82f6;
  --background-color: #ffffff;
  --secondary-color: #f4f4f5;
}

/* Dark mode */
@media (prefers-color-scheme: dark) {
  :root {
    --background-color: #09090b;
    --secondary-color: #27272a;
  }
}
```

## User Flow

1. On load, the available agents are fetched from the backend
2. The user selects an agent from the sidebar
3. The sessions for that agent are loaded
4. The user can:
   - Select an existing session to continue the conversation
   - Create a new session
   - Rename or delete sessions
5. Messages are sent to the backend, which forwards them to the agent
6. The response is received as AG-UI events and displayed via streaming

## Scripts

```bash
npm run dev       # Development with hot reload
npm run build     # Production build
npm run start     # Start production build
npm run lint      # Run ESLint
```

## Development

### Adding a new component

1. Create the component in the appropriate folder
2. Export it if necessary
3. Components must be client components (`"use client"`) if they use hooks

### Modifying the theme

Edit the CSS variables in `globals.css`. Changes are reflected automatically.

### Extending the API client

Add new functions in `lib/api.ts` following the existing pattern.

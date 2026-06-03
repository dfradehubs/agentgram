package a2a

// AgentCard represents the agent-card.json of an A2A agent
type AgentCard struct {
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	URL                string            `json:"url"`
	Provider           AgentProvider     `json:"provider,omitempty"`
	Version            string            `json:"version,omitempty"`
	Capabilities       AgentCapabilities `json:"capabilities,omitempty"`
	Skills             []AgentSkill      `json:"skills,omitempty"`
	DefaultInputModes  []string          `json:"defaultInputModes,omitempty"`  // e.g. ["text", "file"]
	DefaultOutputModes []string          `json:"defaultOutputModes,omitempty"` // e.g. ["text", "file"]
	PreferredTransport string            `json:"preferredTransport,omitempty"` // "jsonrpc", "http+sse"
}

// AgentProvider agent provider information
type AgentProvider struct {
	Organization string `json:"organization,omitempty"`
	URL          string `json:"url,omitempty"`
}

// AgentCapabilities agent capabilities
type AgentCapabilities struct {
	Streaming              bool `json:"streaming,omitempty"`
	PushNotifications      bool `json:"pushNotifications,omitempty"`
	StateTransitionHistory bool `json:"stateTransitionHistory,omitempty"`
}

// AgentSkill represents an agent skill
type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

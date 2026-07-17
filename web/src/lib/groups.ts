interface GroupAgentCandidate {
  id: string;
  require_github_token?: boolean;
}

export function selectGroupAnchorAgent(
  agentIds: string[],
  agents: GroupAgentCandidate[],
  githubConnected: boolean,
): string | undefined {
  if (githubConnected) return agentIds[0];
  return agentIds.find((id) => {
    const agent = agents.find((candidate) => candidate.id === id);
    return agent && !agent.require_github_token;
  })
    ?? agentIds[0];
}

export function groupRequiresGitHubConnection(
  agentIds: string[],
  agents: GroupAgentCandidate[],
  githubConnected: boolean,
): boolean {
  if (githubConnected || agentIds.length === 0) return false;
  const members = agentIds
    .map((id) => agents.find((agent) => agent.id === id))
    .filter((agent): agent is GroupAgentCandidate => !!agent);
  return members.length > 0 && members.every((agent) => agent.require_github_token);
}

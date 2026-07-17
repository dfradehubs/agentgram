import { describe, expect, it } from "vitest";
import { groupRequiresGitHubConnection, selectGroupAnchorAgent } from "../groups";

describe("selectGroupAnchorAgent", () => {
  it("prefers a usable member when the first member requires GitHub", () => {
    expect(selectGroupAnchorAgent(
      ["github-agent", "public-agent"],
      [
        { id: "github-agent", require_github_token: true },
        { id: "public-agent", require_github_token: false },
      ],
      false,
    )).toBe("public-agent");
  });

  it("keeps the first member when GitHub is connected", () => {
    expect(selectGroupAnchorAgent(
      ["github-agent", "public-agent"],
      [{ id: "github-agent", require_github_token: true }],
      true,
    )).toBe("github-agent");
  });
});

describe("groupRequiresGitHubConnection", () => {
  it("does not block a mixed group when one member is usable", () => {
    expect(groupRequiresGitHubConnection(
      ["github-agent", "public-agent"],
      [
        { id: "github-agent", require_github_token: true },
        { id: "public-agent", require_github_token: false },
      ],
      false,
    )).toBe(false);
  });

  it("blocks a group when every known member requires GitHub", () => {
    expect(groupRequiresGitHubConnection(
      ["github-a", "github-b"],
      [
        { id: "github-a", require_github_token: true },
        { id: "github-b", require_github_token: true },
      ],
      false,
    )).toBe(true);
  });
});

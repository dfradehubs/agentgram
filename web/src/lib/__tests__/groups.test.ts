import { describe, expect, it } from "vitest";
import { selectGroupAnchorAgent } from "../groups";

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

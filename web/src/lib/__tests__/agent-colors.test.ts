import { describe, it, expect } from "vitest";
import { getEntityColor } from "../agent-colors";

describe("getEntityColor", () => {
  it("returns an EntityColor object with expected properties", () => {
    const color = getEntityColor("agent-1");

    expect(color).toHaveProperty("border");
    expect(color).toHaveProperty("bg");
    expect(color).toHaveProperty("avatarFrom");
    expect(color).toHaveProperty("avatarTo");
    expect(color).toHaveProperty("iconBg");
    expect(color).toHaveProperty("iconFg");
  });

  it("returns the same color for the same id across calls", () => {
    const first = getEntityColor("my-agent");
    const second = getEntityColor("my-agent");

    expect(first).toStrictEqual(second);
  });

  it("returns different colors for different ids", () => {
    const a = getEntityColor("agent-alpha");
    const b = getEntityColor("agent-beta");

    // While hash collisions are possible, these two strings should differ
    expect(a).not.toStrictEqual(b);
  });

  it("handles empty string without throwing", () => {
    const color = getEntityColor("");

    expect(color).toHaveProperty("border");
    expect(typeof color.avatarFrom).toBe("string");
  });

  it("handles very long ids", () => {
    const longId = "a".repeat(10_000);
    const color = getEntityColor(longId);

    expect(color).toHaveProperty("border");
  });

  it("handles special characters in id", () => {
    const color = getEntityColor("agent/mcp@server#1!$%");

    expect(color).toHaveProperty("avatarFrom");
    expect(color.avatarFrom).toMatch(/^#[0-9a-f]{6}$/);
  });

  it("avatarFrom values are valid hex colors", () => {
    const ids = ["a", "b", "c", "test-agent", "mcp-server-42"];

    for (const id of ids) {
      const color = getEntityColor(id);
      expect(color.avatarFrom).toMatch(/^#[0-9a-f]{6}$/);
      expect(color.avatarTo).toMatch(/^#[0-9a-f]{6}$/);
    }
  });
});

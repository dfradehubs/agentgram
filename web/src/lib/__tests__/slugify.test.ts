import { describe, expect, it } from "vitest";
import { slugify } from "../slugify";

describe("slugify", () => {
  it("turns a name into an agent id", () => {
    expect(slugify("Logs Agent")).toBe("logs-agent");
  });
  it("falls back when empty", () => {
    expect(slugify("   ")).toBe("agent");
  });
});

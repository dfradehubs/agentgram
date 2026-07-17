import { describe, expect, it } from "vitest";
import { debateIncompleteReason } from "../group-events";

describe("debateIncompleteReason", () => {
  it("extracts a supported incomplete reason", () => {
    expect(debateIncompleteReason({
      type: "CUSTOM",
      subType: "debate.incomplete",
      data: { reason: "all_agents_failed" },
    })).toBe("all_agents_failed");
  });

  it("ignores unrelated and malformed events", () => {
    expect(debateIncompleteReason({ type: "RUN_FINISHED" })).toBeNull();
    expect(debateIncompleteReason({ type: "CUSTOM", subType: "debate.incomplete", data: {} })).toBeNull();
  });

  it.each(["persistence_error", "max_turns"])("recognizes %s", (reason) => {
    expect(debateIncompleteReason({
      type: "CUSTOM",
      subType: "debate.incomplete",
      data: { reason },
    })).toBe(reason);
  });
});

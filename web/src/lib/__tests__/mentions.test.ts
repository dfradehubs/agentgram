import { describe, it, expect } from "vitest";
import {
  applyMention,
  extractMentions,
  filterMentionOptions,
  findActiveMention,
  resolveMentions,
} from "../mentions";

const roster = ["logs-agent", "metrics-agent", "logs", "logs.prod"];

describe("extractMentions", () => {
  it("matches a plain mention", () => {
    expect(extractMentions("@logs-agent look at this", roster)).toEqual(["logs-agent"]);
  });

  it("matches a mention with trailing sentence punctuation", () => {
    expect(extractMentions("hey @logs-agent.", roster)).toEqual(["logs-agent"]);
    expect(extractMentions("@metrics-agent, please", roster)).toEqual(["metrics-agent"]);
  });

  it("matches a mention at the start of the text", () => {
    expect(extractMentions("@logs hi", roster)).toEqual(["logs"]);
  });

  it("does not match a mid-word @ (email, not a mention)", () => {
    expect(extractMentions("mail me at me@logs-agent.com", roster)).toEqual([]);
  });

  it("matches an ID containing a dot literally", () => {
    expect(extractMentions("@logs.prod is down", roster)).toEqual(["logs.prod"]);
  });

  it("does not match a shorter ID that is a prefix of a longer dotted ID", () => {
    // "@logs.prod" must NOT resolve to "logs".
    expect(extractMentions("@logs.prod down", roster)).toEqual(["logs.prod"]);
  });

  it("is case-insensitive so a case typo still resolves (no silent widening)", () => {
    // "@Logs-Agent" must resolve to "logs-agent", not match nothing and fall
    // back to broadcasting to the whole group.
    expect(extractMentions("@Logs-Agent here", roster)).toEqual(["logs-agent"]);
    expect(extractMentions("@METRICS-AGENT here", roster)).toEqual(["metrics-agent"]);
  });

  it("does not match an ID that is a prefix of a longer word", () => {
    expect(extractMentions("@logs-agentic thing", roster)).toEqual([]);
  });

  it("returns multiple mentions", () => {
    const got = extractMentions("@logs-agent and @metrics-agent", roster);
    expect(got).toContain("logs-agent");
    expect(got).toContain("metrics-agent");
  });

  it("rejects a case-insensitive collision instead of widening the roster", () => {
    const got = resolveMentions("@logs check this", ["logs", "Logs"]);
    expect(got).toEqual({ agentIds: [], error: "ambiguous" });
  });

  it("rejects an unrecognized explicit mention instead of using the whole roster", () => {
    const got = resolveMentions("ask @log-agent please", roster);
    expect(got).toEqual({ agentIds: [], error: "unrecognized" });
  });

  it("distinguishes plain text from an unresolved mention", () => {
    expect(resolveMentions("check the cluster", roster)).toEqual({ agentIds: [] });
  });
});

describe("mention autocomplete helpers", () => {
  const options = [
    { id: "logs-agent", label: "Log Explorer" },
    { id: "metrics.prod", label: "Production Metrics" },
  ];

  it("finds a mention at the start or after whitespace", () => {
    expect(findActiveMention("@log", 4)).toEqual({ start: 0, end: 4, query: "log" });
    expect(findActiveMention("ask\n@metrics.pr now", 15)).toEqual({ start: 4, end: 15, query: "metrics.pr" });
  });

  it("does not treat an email or mid-word @ as a mention", () => {
    expect(findActiveMention("me@logs", 7)).toBeNull();
    expect(findActiveMention("mail me@example.com", 10)).toBeNull();
  });

  it("filters by id or display label case-insensitively", () => {
    expect(filterMentionOptions(options, "PROD").map((option) => option.id)).toEqual(["metrics.prod"]);
    expect(filterMentionOptions(options, "explorer").map((option) => option.id)).toEqual(["logs-agent"]);
  });

  it("replaces the complete active token and restores the caret", () => {
    const active = findActiveMention("ask @met.prod please", 8);
    expect(active).not.toBeNull();
    expect(applyMention("ask @met.prod please", active!, "metrics.prod")).toEqual({
      value: "ask @metrics.prod please",
      caret: 17,
    });
  });

  it("adds a trailing space at the end of the message", () => {
    const active = findActiveMention("@logs-a", 7)!;
    expect(applyMention("@logs-a", active, "logs-agent")).toEqual({
      value: "@logs-agent ",
      caret: 12,
    });
  });
});

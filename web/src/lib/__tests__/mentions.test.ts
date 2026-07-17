import { describe, it, expect } from "vitest";
import { extractMentions } from "../mentions";

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
});

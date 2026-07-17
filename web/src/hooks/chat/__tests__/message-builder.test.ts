import { describe, expect, it } from "vitest";
import { applyFinalMessage, buildTimelineFromMessages } from "../message-builder";
import type { ChartData, Message, TimelineItem } from "@/lib/types";

const chart: ChartData = {
  chartType: "bar",
  title: "Requests",
  labels: ["ok"],
  datasets: [{ label: "requests", data: [3] }],
};

describe("chart-only assistant messages", () => {
  it("rebuilds a persisted chart without requiring tool calls", () => {
    const items = buildTimelineFromMessages([{
      role: "assistant",
      content: "",
      agent_id: "metrics-agent",
      content_parts: [{ type: "chart", chart }],
    }]);

    expect(items).toEqual([{ type: "chart", chart, agentId: "metrics-agent" }]);
  });

  it("adds chart content parts when finalizing a structured-only turn", () => {
    const message: Message = { role: "assistant", content: "", agent_id: "metrics-agent" };
    const items: TimelineItem[] = [{ type: "chart", chart, agentId: "metrics-agent" }];

    const finalized = applyFinalMessage(items, message);

    expect(finalized.at(-1)).toMatchObject({
      type: "message",
      message: { content_parts: [{ type: "chart", chart }] },
    });
  });
});

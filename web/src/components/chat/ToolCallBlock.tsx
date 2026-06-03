"use client";

import { memo, useState, useCallback, useMemo } from "react";
import type { ToolCall } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { ChevronDown, ChevronRight, Loader2, Wrench, BarChart2, Sparkles } from "lucide-react";
import { detectChartData } from "@/lib/chart-detection";
import type { ChartData } from "@/lib/types";
import { ChartBlock } from "./ChartBlock";

function formatContent(raw: string): string {
  try {
    const parsed = JSON.parse(raw);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return raw;
  }
}

export const ToolCallBlock = memo(function ToolCallBlock({
  toolCall,
  serverLabel,
}: {
  toolCall: ToolCall;
  serverLabel?: string;
}) {
  const [isExpanded, setIsExpanded] = useState(false);
  const [chartData, setChartData] = useState<ChartData | null>(null);
  const [showChart, setShowChart] = useState(false);
  const [isExtractingChart, setIsExtractingChart] = useState(false);
  const [extractError, setExtractError] = useState<string | null>(null);

  // Detect chartable data from the result (heuristic, instant, memoized)
  const detectedChart = useMemo(
    () => toolCall.result ? detectChartData(toolCall.result) : null,
    [toolCall.result]
  );

  const handleShowChart = useCallback(() => {
    if (detectedChart) {
      setChartData(detectedChart);
      setShowChart(true);
    }
  }, [detectedChart]);

  const handleExtractChart = useCallback(async () => {
    if (!toolCall.result) return;
    setIsExtractingChart(true);
    setExtractError(null);
    try {
      const resp = await fetch("/api/chart/extract", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ data: toolCall.result }),
      });
      if (resp.status === 501) {
        setExtractError("Chart extraction not configured");
        return;
      }
      if (!resp.ok) {
        setExtractError("Error extracting chart");
        return;
      }
      const body = await resp.json();
      if (body.chart) {
        setChartData(body.chart);
        setShowChart(true);
      } else {
        setExtractError("Could not extract chartable data");
      }
    } catch {
      setExtractError("Connection error");
    } finally {
      setIsExtractingChart(false);
    }
  }, [toolCall.result]);

  return (
    <div className="rounded-lg border bg-muted/30 text-xs">
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left"
      >
        <Wrench className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="font-medium">{toolCall.toolName}</span>
        {serverLabel && (
          <Badge variant="outline" className="h-4 px-1.5 text-[10px]">
            {serverLabel}
          </Badge>
        )}
        {!toolCall.isComplete && (
          <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
        )}
        {toolCall.isComplete && (
          <Badge variant="secondary" className="ml-auto h-4 px-1.5 text-[10px]">
            completed
          </Badge>
        )}
        {isExpanded ? (
          <ChevronDown className="ml-auto h-3 w-3 text-muted-foreground" />
        ) : (
          <ChevronRight className="ml-auto h-3 w-3 text-muted-foreground" />
        )}
      </button>
      {isExpanded && (
        <div className="border-t px-3 py-2">
          {toolCall.args && (
            <div className="mb-2">
              <span className="font-medium text-muted-foreground">Args:</span>
              <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded bg-background p-2 text-[11px]">
                {formatContent(toolCall.args)}
              </pre>
            </div>
          )}
          {toolCall.result && (
            <div>
              <span className="font-medium text-muted-foreground">Result:</span>
              <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded bg-background p-2 text-[11px]">
                {formatContent(toolCall.result)}
              </pre>
            </div>
          )}
          {/* Chart buttons */}
          {toolCall.isComplete && toolCall.result && !showChart && (
            <div className="mt-2 flex items-center gap-2">
              {detectedChart && (
                <button
                  onClick={handleShowChart}
                  className="flex items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <BarChart2 className="h-3 w-3" />
                  View as chart
                </button>
              )}
              {!detectedChart && (
                <button
                  onClick={handleExtractChart}
                  disabled={isExtractingChart}
                  className="flex items-center gap-1.5 rounded-md border border-dashed px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
                >
                  {isExtractingChart ? (
                    <Loader2 className="h-3 w-3 animate-spin" />
                  ) : (
                    <Sparkles className="h-3 w-3" />
                  )}
                  Generate chart with AI
                </button>
              )}
              {extractError && (
                <span className="text-[10px] text-muted-foreground">{extractError}</span>
              )}
            </div>
          )}
          {/* Rendered chart */}
          {showChart && chartData && (
            <div className="mt-2">
              <ChartBlock chart={chartData} />
            </div>
          )}
        </div>
      )}
    </div>
  );
});

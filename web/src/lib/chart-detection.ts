// Heuristic chart detection for tool result data.
// Analyzes JSON strings and attempts to extract chartable data.

import type { ChartData } from "@/lib/types";
export type { ChartData } from "@/lib/types";

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isNumeric(v: unknown): v is number {
  return typeof v === "number" && isFinite(v);
}

const DATE_PATTERNS = [
  /^\d{4}-\d{2}-\d{2}/, // ISO date
  /^\d{2}\/\d{2}\/\d{4}/, // DD/MM/YYYY or MM/DD/YYYY
  /^\d{4}\/\d{2}\/\d{2}/, // YYYY/MM/DD
  /^\w{3}\s+\d{1,2},?\s+\d{4}/, // Mon DD, YYYY
  /^\d{4}-\d{2}-\d{2}T/, // ISO datetime
];

function isDateLike(value: string): boolean {
  return DATE_PATTERNS.some((re) => re.test(value));
}

function extractNumericFields(obj: Record<string, unknown>): string[] {
  return Object.keys(obj).filter((k) => isNumeric(obj[k]));
}

function extractStringFields(obj: Record<string, unknown>): string[] {
  return Object.keys(obj).filter((k) => typeof obj[k] === "string");
}

function extractLabelField(obj: Record<string, unknown>): string | null {
  const stringFields = extractStringFields(obj);
  if (stringFields.length === 0) return null;

  // Prefer common label field names
  const preferred = ["name", "label", "category", "key", "id", "title", "month", "year", "date", "day", "period", "grupo", "nombre", "etiqueta", "categoria"];
  for (const pref of preferred) {
    const match = stringFields.find((f) => f.toLowerCase() === pref);
    if (match) return match;
  }
  return stringFields[0];
}

function detectDateField(obj: Record<string, unknown>): string | null {
  const stringFields = extractStringFields(obj);
  for (const field of stringFields) {
    const val = obj[field];
    if (typeof val === "string" && isDateLike(val)) return field;
  }
  return null;
}

/**
 * Try to detect chartable data from a parsed value.
 * Returns ChartData if patterns match, null otherwise.
 */
function detectFromParsed(data: unknown): ChartData | null {
  // Case 1: Plain object with all numeric values → pie chart
  if (isRecord(data)) {
    const keys = Object.keys(data);
    if (keys.length >= 2 && keys.length <= 30) {
      const numericKeys = keys.filter((k) => isNumeric(data[k]));
      if (numericKeys.length === keys.length) {
        return {
          chartType: "pie",
          labels: keys,
          datasets: [{ label: "Valor", data: keys.map((k) => data[k] as number) }],
        };
      }
    }
  }

  // Case 2: Array of objects
  if (Array.isArray(data) && data.length >= 2 && data.every(isRecord)) {
    const sample = data[0] as Record<string, unknown>;
    const numericFields = extractNumericFields(sample);
    if (numericFields.length === 0) return null;

    // Check all items have the same numeric fields
    const validItems = data.filter((item) => {
      const rec = item as Record<string, unknown>;
      return numericFields.every((f) => isNumeric(rec[f]));
    });
    if (validItems.length < 2) return null;

    const items = validItems as Record<string, unknown>[];

    // Detect date field → line chart
    const dateField = detectDateField(sample);
    if (dateField) {
      return {
        chartType: "line",
        labels: items.map((item) => String(item[dateField])),
        xAxisLabel: dateField,
        datasets: numericFields.map((field) => ({
          label: field,
          data: items.map((item) => item[field] as number),
        })),
        options: { showLegend: numericFields.length > 1 },
      };
    }

    // Detect label field
    const labelField = extractLabelField(sample);
    const labels = labelField
      ? items.map((item) => String(item[labelField]))
      : items.map((_, i) => String(i + 1));

    // Single numeric field → simple bar chart
    if (numericFields.length === 1) {
      return {
        chartType: "bar",
        labels,
        ...(labelField ? { xAxisLabel: labelField } : {}),
        yAxisLabel: numericFields[0],
        datasets: [{
          label: numericFields[0],
          data: items.map((item) => item[numericFields[0]] as number),
        }],
      };
    }

    // Multiple numeric fields → grouped bar chart
    return {
      chartType: "bar",
      labels,
      ...(labelField ? { xAxisLabel: labelField } : {}),
      datasets: numericFields.map((field) => ({
        label: field,
        data: items.map((item) => item[field] as number),
      })),
      options: { showLegend: true },
    };
  }

  // Case 3: Simple array of numbers → bar chart with indices
  if (Array.isArray(data) && data.length >= 2 && data.every(isNumeric)) {
    return {
      chartType: "bar",
      labels: data.map((_, i) => String(i + 1)),
      datasets: [{ label: "Valor", data: data as number[] }],
    };
  }

  return null;
}

/**
 * Analyze a JSON string (tool result) and attempt to extract chart data.
 * Returns ChartData if chartable patterns are detected, null otherwise.
 */
export function detectChartData(jsonStr: string): ChartData | null {
  if (!jsonStr || jsonStr.length < 5) return null;

  try {
    const data = JSON.parse(jsonStr);
    return detectFromParsed(data);
  } catch {
    return null;
  }
}

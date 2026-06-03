"use client";

import { useState, useEffect, useCallback, useRef } from "react";

interface UseMetricsResult<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

/**
 * Hook for fetching metrics data. Re-fetches when `key` changes.
 * Keeps stale data visible while fetching new data (Grafana-like behavior).
 * Only shows loading=true on the very first fetch (when there's no data yet).
 */
export function useMetrics<T>(fetchFn: () => Promise<T>, key: string, pollingMs?: number): UseMetricsResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const fetchRef = useRef(fetchFn);
  const hasDataRef = useRef(false);
  fetchRef.current = fetchFn;

  const load = useCallback(async () => {
    try {
      // Only show loading spinner on initial load (no existing data)
      if (!hasDataRef.current) setLoading(true);
      setError(null);
      const result = await fetchRef.current();
      hasDataRef.current = true;
      setData(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setLoading(false);
    }
  }, [key]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!pollingMs) return;
    const interval = setInterval(load, pollingMs);
    return () => clearInterval(interval);
  }, [load, pollingMs]);

  return { data, loading, error, refresh: load };
}

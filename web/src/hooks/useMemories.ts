import { useState, useEffect, useCallback, useRef } from "react";
import type { Memory } from "../types";

export function useMemories(agentFilter?: string) {
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  const fetchMemories = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      if (agentFilter) params.set("agent", agentFilter);
      const res = await fetch(`/api/memories?${params}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as Memory[];
      setMemories(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch memories");
    } finally {
      setLoading(false);
    }
  }, [agentFilter]);

  // Initial fetch
  useEffect(() => {
    void fetchMemories();
  }, [fetchMemories]);

  // Listen for memory_* events on the SSE stream to auto-refresh
  useEffect(() => {
    let cancelled = false;

    const es = new EventSource("/api/activity/stream");
    eventSourceRef.current = es;

    es.onmessage = (event) => {
      try {
        const entry = JSON.parse(event.data) as { event_type: string };
        if (entry.event_type.startsWith("memory_") && !cancelled) {
          void fetchMemories();
        }
      } catch {
        // ignore
      }
    };

    es.onerror = () => {
      es.close();
    };

    return () => {
      cancelled = true;
      es.close();
    };
  }, [fetchMemories]);

  return { memories, loading, error, refetch: fetchMemories };
}

import { useState, useEffect, useRef, useCallback } from "react";
import type { ActivityEntry } from "../types";

const POLL_INTERVAL = 5000;
const RECONNECT_DELAY = 3000;

export function useActivityStream(agentFilter?: string) {
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const addEntry = useCallback((entry: ActivityEntry) => {
    setEntries((prev) => {
      if (prev.some((e) => e.id === entry.id)) return prev;
      return [...prev, entry];
    });
  }, []);

  // SSE connection
  useEffect(() => {
    let cancelled = false;

    function connect() {
      if (cancelled) return;

      const es = new EventSource("/api/activity/stream");
      eventSourceRef.current = es;

      es.onopen = () => {
        if (!cancelled) setConnected(true);
      };

      es.onmessage = (event) => {
        try {
          const entry = JSON.parse(event.data) as ActivityEntry;
          addEntry(entry);
        } catch {
          // ignore malformed messages
        }
      };

      es.onerror = () => {
        es.close();
        if (!cancelled) {
          setConnected(false);
          reconnectTimer.current = setTimeout(connect, RECONNECT_DELAY);
        }
      };
    }

    connect();

    return () => {
      cancelled = true;
      eventSourceRef.current?.close();
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
    };
  }, [addEntry]);

  // Fallback: initial fetch + polling when SSE not connected
  useEffect(() => {
    let timer: ReturnType<typeof setInterval>;

    async function fetchActivity() {
      try {
        const params = new URLSearchParams({ limit: "100" });
        if (agentFilter) params.set("agent", agentFilter);
        const res = await fetch(`/api/activity?${params}`);
        if (!res.ok) return;
        const data = (await res.json()) as ActivityEntry[];
        data.forEach(addEntry);
      } catch {
        // silently retry on next poll
      }
    }

    // Always do an initial fetch for history
    void fetchActivity();

    if (!connected) {
      timer = setInterval(fetchActivity, POLL_INTERVAL);
    }

    return () => clearInterval(timer);
  }, [connected, agentFilter, addEntry]);

  // Filter by agent if specified
  const filtered = agentFilter
    ? entries.filter((e) => e.agent_name === agentFilter)
    : entries;

  // Sort by timestamp
  const sorted = [...filtered].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  );

  return { entries: sorted, connected };
}

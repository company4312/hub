import { useState, useEffect } from "react";
import type { Agent } from "../types";

export function useAgents() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchAgents() {
      try {
        const res = await fetch("/api/agents");
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = (await res.json()) as Agent[];
        if (!cancelled) {
          setAgents(data);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Failed to fetch agents");
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void fetchAgents();
    return () => {
      cancelled = true;
    };
  }, []);

  return { agents, loading, error };
}

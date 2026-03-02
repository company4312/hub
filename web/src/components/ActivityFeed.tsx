import { useRef, useEffect, useState } from "react";
import type { ActivityEntry as ActivityEntryType } from "../types";
import { ActivityEntry } from "./ActivityEntry";

interface Props {
  entries: ActivityEntryType[];
  connected: boolean;
  selectedAgent: string | null;
  agentTitle?: string;
}

function AgentStats({ entries }: { entries: ActivityEntryType[] }) {
  const messageCount = entries.filter(
    (e) =>
      e.event_type === "message_sent" ||
      e.event_type === "message_received" ||
      e.event_type === "agent_message",
  ).length;

  const lastActive = entries.length > 0 ? entries[entries.length - 1]!.timestamp : null;

  return (
    <div className="flex gap-6 px-4 py-3 bg-gray-900/50 border-b border-gray-800 text-sm">
      <div>
        <span className="text-gray-500">Messages:</span>{" "}
        <span className="text-gray-200 font-medium">{messageCount}</span>
      </div>
      {lastActive && (
        <div>
          <span className="text-gray-500">Last active:</span>{" "}
          <span className="text-gray-200 font-medium">
            {new Date(lastActive).toLocaleString()}
          </span>
        </div>
      )}
    </div>
  );
}

export function ActivityFeed({ entries, connected, selectedAgent, agentTitle }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [paused, setPaused] = useState(false);

  // Auto-scroll to bottom when new entries arrive
  useEffect(() => {
    if (!paused && bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [entries.length, paused]);

  // Detect manual scroll to auto-pause
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    function handleScroll() {
      if (!container) return;
      const { scrollTop, scrollHeight, clientHeight } = container;
      const atBottom = scrollHeight - scrollTop - clientHeight < 50;
      if (atBottom && paused) setPaused(false);
    }

    container.addEventListener("scroll", handleScroll);
    return () => container.removeEventListener("scroll", handleScroll);
  }, [paused]);

  return (
    <div className="flex-1 flex flex-col min-w-0">
      {/* Header */}
      <header className="px-4 py-3 border-b border-gray-800 bg-gray-900/80 backdrop-blur-sm flex items-center justify-between shrink-0">
        <div>
          <h2 className="text-base font-semibold text-white">
            {selectedAgent ?? "All Activity"}
          </h2>
          {agentTitle && (
            <p className="text-xs text-gray-500 mt-0.5">{agentTitle}</p>
          )}
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setPaused((p) => !p)}
            className={`text-xs px-3 py-1.5 rounded-md font-medium transition-colors ${
              paused
                ? "bg-amber-500/20 text-amber-400 hover:bg-amber-500/30"
                : "bg-gray-800 text-gray-400 hover:text-gray-200"
            }`}
          >
            {paused ? "▶ Resume" : "⏸ Pause"}
          </button>
          <span
            className={`inline-block w-2 h-2 rounded-full ${
              connected ? "bg-green-500" : "bg-red-500"
            }`}
            title={connected ? "Connected (SSE)" : "Disconnected (polling)"}
          />
        </div>
      </header>

      {/* Agent stats when filtered */}
      {selectedAgent && <AgentStats entries={entries} />}

      {/* Feed */}
      <div ref={containerRef} className="flex-1 overflow-y-auto">
        {entries.length === 0 ? (
          <div className="flex items-center justify-center h-full text-gray-600 text-sm">
            No activity yet — waiting for events…
          </div>
        ) : (
          entries.map((entry) => (
            <ActivityEntry
              key={entry.id}
              id={entry.id}
              timestamp={entry.timestamp}
              agentName={entry.agent_name}
              eventType={entry.event_type}
              content={entry.content}
            />
          ))
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

import type { EventType } from "../types";

const EVENT_COLORS: Record<EventType, { bg: string; text: string }> = {
  message_sent: { bg: "bg-blue-500/20", text: "text-blue-400" },
  message_received: { bg: "bg-green-500/20", text: "text-green-400" },
  agent_message: { bg: "bg-purple-500/20", text: "text-purple-400" },
  session_created: { bg: "bg-gray-500/20", text: "text-gray-400" },
  session_destroyed: { bg: "bg-gray-500/20", text: "text-gray-400" },
  memory_created: { bg: "bg-amber-500/20", text: "text-amber-400" },
  memory_updated: { bg: "bg-amber-500/20", text: "text-amber-400" },
  memory_deleted: { bg: "bg-amber-500/20", text: "text-amber-400" },
  error: { bg: "bg-red-500/20", text: "text-red-400" },
};

const AGENT_COLORS = [
  "bg-blue-500",
  "bg-green-500",
  "bg-purple-500",
  "bg-amber-500",
  "bg-rose-500",
  "bg-cyan-500",
  "bg-indigo-500",
  "bg-emerald-500",
];

function agentColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash);
  }
  return AGENT_COLORS[Math.abs(hash) % AGENT_COLORS.length]!;
}

function formatTime(ts: string): string {
  const d = new Date(ts);
  return d.toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function formatLabel(eventType: string): string {
  return eventType.replace(/_/g, " ");
}

interface Props {
  id: string;
  timestamp: string;
  agentName: string;
  eventType: EventType;
  content: string;
}

export function ActivityEntry({ timestamp, agentName, eventType, content }: Props) {
  const colors = EVENT_COLORS[eventType] ?? EVENT_COLORS.error;

  return (
    <div className="flex items-start gap-3 px-4 py-3 hover:bg-gray-800/50 transition-colors border-b border-gray-800/50">
      {/* Agent avatar */}
      <div
        className={`${agentColor(agentName)} w-8 h-8 rounded-full flex items-center justify-center shrink-0 text-sm font-bold text-white`}
      >
        {agentName.charAt(0).toUpperCase()}
      </div>

      <div className="flex-1 min-w-0">
        {/* Header row */}
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-semibold text-sm text-gray-100">{agentName}</span>
          <span
            className={`${colors.bg} ${colors.text} text-xs px-2 py-0.5 rounded-full font-medium`}
          >
            {formatLabel(eventType)}
          </span>
          <span className="text-xs text-gray-500 font-mono ml-auto shrink-0">
            {formatTime(timestamp)}
          </span>
        </div>

        {/* Content */}
        {content && (
          <p className="mt-1 text-sm text-gray-300 whitespace-pre-wrap break-words font-mono leading-relaxed">
            {content}
          </p>
        )}
      </div>
    </div>
  );
}

import type { EventType } from "../types";

const EVENT_COLORS: Record<EventType, { bg: string; text: string; icon: string }> = {
  message_sent: { bg: "bg-blue-500/20", text: "text-blue-400", icon: "💬" },
  message_received: { bg: "bg-green-500/20", text: "text-green-400", icon: "📨" },
  agent_message: { bg: "bg-purple-500/20", text: "text-purple-400", icon: "🤝" },
  session_created: { bg: "bg-gray-500/20", text: "text-gray-400", icon: "🔌" },
  session_destroyed: { bg: "bg-gray-500/20", text: "text-gray-400", icon: "⏏" },
  memory_created: { bg: "bg-amber-500/20", text: "text-amber-400", icon: "🧠" },
  memory_updated: { bg: "bg-amber-500/20", text: "text-amber-400", icon: "🧠" },
  memory_deleted: { bg: "bg-amber-500/20", text: "text-amber-400", icon: "🧠" },
  project_created: { bg: "bg-teal-500/20", text: "text-teal-400", icon: "📁" },
  task_created: { bg: "bg-indigo-500/20", text: "text-indigo-400", icon: "📋" },
  task_status_changed: { bg: "bg-cyan-500/20", text: "text-cyan-400", icon: "🔄" },
  task_assigned: { bg: "bg-rose-500/20", text: "text-rose-400", icon: "👤" },
  task_comment: { bg: "bg-violet-500/20", text: "text-violet-400", icon: "💭" },
  error: { bg: "bg-red-500/20", text: "text-red-400", icon: "❌" },
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

function parseMetadata(raw: string): Record<string, string> {
  if (!raw) return {};
  try {
    return JSON.parse(raw) as Record<string, string>;
  } catch {
    return {};
  }
}

function MetadataContext({ eventType, metadata }: { eventType: EventType; metadata: Record<string, string> }) {
  switch (eventType) {
    case "agent_message": {
      const to = metadata.to;
      return to ? (
        <span className="text-xs text-gray-500">
          → <span className="text-purple-400 font-medium">{to}</span>
        </span>
      ) : null;
    }
    case "task_created":
    case "task_status_changed":
    case "task_assigned":
    case "task_comment": {
      const parts: string[] = [];
      if (metadata.task_id) parts.push(metadata.task_id);
      if (metadata.status) parts.push(`→ ${metadata.status}`);
      if (metadata.assignee) parts.push(`→ ${metadata.assignee}`);
      return parts.length > 0 ? (
        <span className="text-xs text-gray-500 font-mono">{parts.join(" ")}</span>
      ) : null;
    }
    case "project_created": {
      return metadata.project_id ? (
        <span className="text-xs text-gray-500 font-mono">{metadata.project_id}</span>
      ) : null;
    }
    case "memory_created": {
      return metadata.category ? (
        <span className="text-xs text-gray-500">[{metadata.category}]</span>
      ) : null;
    }
    default:
      return null;
  }
}

interface Props {
  id: string;
  timestamp: string;
  agentName: string;
  eventType: EventType;
  content: string;
  metadata?: string;
}

export function ActivityEntry({ timestamp, agentName, eventType, content, metadata }: Props) {
  const colors = EVENT_COLORS[eventType] ?? EVENT_COLORS.error;
  const meta = parseMetadata(metadata ?? "");

  // For message_received, show as a chat-style response
  const isResponse = eventType === "message_received";
  // For agent_message, show as an inter-agent conversation
  const isInterAgent = eventType === "agent_message";

  return (
    <div className={`flex items-start gap-3 px-4 py-3 hover:bg-gray-800/50 transition-colors border-b border-gray-800/50 ${isResponse ? "bg-gray-900/30" : ""}`}>
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
          {isInterAgent && meta.to && (
            <span className="text-xs text-gray-500">→ <span className="text-purple-400 font-medium">{meta.to}</span></span>
          )}
          <span
            className={`${colors.bg} ${colors.text} text-xs px-2 py-0.5 rounded-full font-medium inline-flex items-center gap-1`}
          >
            <span>{colors.icon}</span>
            {formatLabel(eventType)}
          </span>
          <MetadataContext eventType={eventType} metadata={meta} />
          <span className="text-xs text-gray-500 font-mono ml-auto shrink-0">
            {formatTime(timestamp)}
          </span>
        </div>

        {/* Content */}
        {content && (
          <p className={`mt-1 text-sm whitespace-pre-wrap break-words leading-relaxed ${
            isResponse ? "text-green-300/90 font-mono" : "text-gray-300 font-mono"
          }`}>
            {content.length > 500 ? content.slice(0, 500) + "…" : content}
          </p>
        )}
      </div>
    </div>
  );
}

import { useEffect } from 'react';
import { useSessionEvents } from '../hooks/useSessionEvents';

const eventTypeLabels: Record<string, string> = {
  tool_call: '🔧 Tool call',
  tool_start: '▶ Tool start',
  tool_complete: '✓ Tool done',
  intent: '💭 Intent',
  reasoning: '🧠 Reasoning',
};

const eventTypeColors: Record<string, string> = {
  tool_call: 'text-blue-400',
  tool_start: 'text-cyan-400',
  tool_complete: 'text-green-400',
  intent: 'text-yellow-400',
  reasoning: 'text-purple-400',
};

interface Props {
  messageId: number;
}

export function SessionEventList({ messageId }: Props) {
  const { events, loading, fetchEvents } = useSessionEvents(messageId);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  if (loading) {
    return <div className="text-xs text-gray-500 mt-1 ml-2">Loading events…</div>;
  }

  if (events.length === 0) {
    return <div className="text-xs text-gray-500 mt-1 ml-2">No internal events recorded.</div>;
  }

  return (
    <div className="mt-1 ml-2 border-l-2 border-gray-700 pl-3 space-y-1">
      {events.map(e => {
        const label = eventTypeLabels[e.event_type] || e.event_type;
        const colorClass = eventTypeColors[e.event_type] || 'text-gray-400';
        return (
          <div key={e.id} className="text-xs">
            <span className={`font-mono ${colorClass}`}>{label}</span>
            <span className="text-gray-500 ml-2">{e.content}</span>
          </div>
        );
      })}
    </div>
  );
}

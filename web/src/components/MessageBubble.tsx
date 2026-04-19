import { useState } from 'react';
import type { Message } from '../types';
import { SessionEventList } from './SessionEventList';

function hashColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash);
  const colors = ['#3b82f6', '#8b5cf6', '#10b981', '#f59e0b', '#ef4444', '#06b6d4', '#ec4899'];
  return colors[Math.abs(hash) % colors.length]!;
}

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

interface Props {
  message: Message;
}

export function MessageBubble({ message }: Props) {
  const [expanded, setExpanded] = useState(false);
  const isUser = message.message_type === 'user_message';
  const isSystem = message.message_type === 'system';
  const hasSession = !!message.copilot_session_id;

  if (isSystem) {
    return (
      <div className="flex justify-center py-1">
        <span className="text-xs text-gray-500 italic">{message.content}</span>
      </div>
    );
  }

  const color = hashColor(message.from_name);

  return (
    <div className={`flex gap-2.5 py-1.5 ${isUser ? 'flex-row-reverse' : ''}`}>
      {/* Avatar */}
      <div
        className="w-8 h-8 rounded-full flex items-center justify-center text-white text-xs font-bold flex-shrink-0 mt-0.5"
        style={{ backgroundColor: color }}
      >
        {message.from_name.slice(0, 2).toUpperCase()}
      </div>

      {/* Content */}
      <div className={`max-w-[75%] ${isUser ? 'items-end' : 'items-start'} flex flex-col`}>
        {/* Header */}
        <div className="flex items-center gap-2 mb-0.5">
          <span className="text-xs font-semibold" style={{ color }}>
            {message.from_name}
          </span>
          {message.to_name && (
            <>
              <span className="text-xs text-gray-600">→</span>
              <span className="text-xs font-semibold" style={{ color: hashColor(message.to_name) }}>
                @{message.to_name}
              </span>
            </>
          )}
          <span className="text-xs text-gray-600">{formatTime(message.created_at)}</span>
        </div>

        {/* Message body */}
        <div
          className={`rounded-lg px-3 py-2 text-sm leading-relaxed ${
            isUser
              ? 'bg-blue-600/20 text-gray-100'
              : 'bg-gray-800 text-gray-200'
          }`}
        >
          <div className="whitespace-pre-wrap break-words">{message.content}</div>
        </div>

        {/* Expand button for session events */}
        {hasSession && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="text-xs text-gray-500 hover:text-gray-300 mt-1 flex items-center gap-1"
          >
            <span>{expanded ? '▼' : '▶'}</span>
            <span>{expanded ? 'Hide' : 'Show'} internal events</span>
          </button>
        )}

        {expanded && hasSession && (
          <SessionEventList messageId={message.id} />
        )}
      </div>
    </div>
  );
}

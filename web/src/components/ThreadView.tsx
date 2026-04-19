import { useEffect, useRef } from 'react';
import type { Message } from '../types';
import { MessageBubble } from './MessageBubble';

interface Props {
  messages: Message[];
  threadTitle: string;
  loading: boolean;
}

export function ThreadView({ messages, threadTitle, loading }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages.length]);

  return (
    <div className="flex-1 flex flex-col h-full">
      <div className="p-4 border-b border-gray-800 flex-shrink-0">
        <h2 className="text-base font-semibold text-gray-100 truncate">{threadTitle}</h2>
      </div>
      <div className="flex-1 overflow-y-auto p-4 space-y-1">
        {loading && messages.length === 0 && (
          <p className="text-gray-500 text-sm text-center mt-8">Loading messages…</p>
        )}
        {messages.map(msg => (
          <MessageBubble key={msg.id} message={msg} />
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

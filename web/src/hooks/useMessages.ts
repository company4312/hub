import { useState, useEffect, useCallback } from 'react';
import type { Message } from '../types';

export function useMessages(threadId: string | null) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(false);

  const refetch = useCallback(() => {
    if (!threadId) return;
    setLoading(true);
    fetch(`/api/threads/${threadId}/messages`)
      .then(r => r.json())
      .then(setMessages)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [threadId]);

  useEffect(() => {
    if (!threadId) {
      setMessages([]);
      return;
    }
    refetch();

    // Subscribe to SSE for new messages.
    const es = new EventSource('/api/threads/stream');
    es.onmessage = (event) => {
      try {
        const msg: Message = JSON.parse(event.data);
        if (msg.thread_id === threadId) {
          setMessages(prev => {
            if (prev.some(m => m.id === msg.id)) return prev;
            return [...prev, msg];
          });
        }
      } catch {}
    };

    return () => es.close();
  }, [threadId, refetch]);

  return { messages, loading, refetch };
}

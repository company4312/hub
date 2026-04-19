import { useState, useEffect, useCallback } from 'react';
import type { Thread, Message } from '../types';

export function useThreads() {
  const [threads, setThreads] = useState<Thread[]>([]);
  const [loading, setLoading] = useState(true);

  const refetch = useCallback(() => {
    fetch('/api/threads')
      .then(r => r.json())
      .then(setThreads)
      .catch(() => {});
  }, []);

  useEffect(() => {
    refetch();
    setLoading(false);

    // Subscribe to SSE for real-time updates.
    const es = new EventSource('/api/threads/stream');
    es.onmessage = (event) => {
      try {
        const msg: Message = JSON.parse(event.data);
        // A new message arrived — refetch threads to update order and preview.
        setThreads(prev => {
          const existing = prev.find(t => t.id === msg.thread_id);
          if (existing) {
            // Move updated thread to top with new last_message.
            return [
              { ...existing, last_message: msg, updated_at: msg.created_at },
              ...prev.filter(t => t.id !== msg.thread_id),
            ];
          }
          // New thread — refetch to get it.
          refetch();
          return prev;
        });
      } catch {}
    };

    return () => es.close();
  }, [refetch]);

  return { threads, loading, refetch };
}

import { useState, useCallback } from 'react';
import type { SessionEvent } from '../types';

export function useSessionEvents(messageId: number | null) {
  const [events, setEvents] = useState<SessionEvent[]>([]);
  const [loading, setLoading] = useState(false);

  const fetch_ = useCallback(() => {
    if (!messageId) return;
    setLoading(true);
    fetch(`/api/messages/${messageId}/events`)
      .then(r => r.json())
      .then(setEvents)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [messageId]);

  return { events, loading, fetchEvents: fetch_ };
}

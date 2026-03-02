export interface Agent {
  name: string;
  title: string;
}

export type EventType =
  | "message_sent"
  | "message_received"
  | "agent_message"
  | "session_created"
  | "session_destroyed"
  | "memory_created"
  | "memory_updated"
  | "memory_deleted"
  | "error";

export interface ActivityEntry {
  id: string;
  timestamp: string;
  agent_name: string;
  event_type: EventType;
  content: string;
  metadata: Record<string, unknown>;
  chat_id: string;
}

export interface Memory {
  id: number;
  agent_name: string;
  category: string;
  content: string;
  source: string;
  created_at: string;
  updated_at: string;
}

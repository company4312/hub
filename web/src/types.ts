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

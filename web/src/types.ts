export interface Agent {
  name: string;
  title: string;
  system_prompt: string;
  model: string;
}

export interface Thread {
  id: string;
  chat_id: number;
  title: string;
  status: string;
  created_at: string;
  updated_at: string;
  last_message?: Message;
}

export interface Message {
  id: number;
  thread_id: string;
  from_name: string;
  to_name?: string;
  content: string;
  message_type: 'user_message' | 'agent_message' | 'system';
  copilot_session_id?: string;
  parent_message_id?: number;
  metadata?: string;
  created_at: string;
}

export interface SessionEvent {
  id: number;
  copilot_session_id: string;
  thread_id: string;
  agent_name: string;
  event_type: string;
  content: string;
  metadata?: string;
  created_at: string;
}

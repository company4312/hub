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
  | "project_created"
  | "task_created"
  | "task_status_changed"
  | "task_assigned"
  | "task_comment"
  | "error";

export interface ActivityEntry {
  id: string;
  timestamp: string;
  agent_name: string;
  event_type: EventType;
  content: string;
  metadata: string;
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

export interface Project {
  id: string;
  name: string;
  description: string;
  status: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export type TaskStatus = "backlog" | "todo" | "in_progress" | "review" | "done";

export type TaskPriority = 1 | 2 | 3 | 4;

export interface Task {
  id: string;
  project_id: string;
  title: string;
  description: string;
  status: TaskStatus;
  assigned_to: string | null;
  created_by: string;
  priority: TaskPriority;
  created_at: string;
  updated_at: string;
}

export interface TaskComment {
  id: number;
  task_id: string;
  agent_name: string;
  content: string;
  created_at: string;
}

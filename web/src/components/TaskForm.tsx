import { useState } from "react";
import type { Agent, TaskPriority } from "../types";

const PRIORITY_OPTIONS: { value: TaskPriority; label: string }[] = [
  { value: 1, label: "Critical" },
  { value: 2, label: "High" },
  { value: 3, label: "Medium" },
  { value: 4, label: "Low" },
];

interface Props {
  projectId: string;
  agents: Agent[];
  onSubmit: (data: {
    id: string;
    title: string;
    description: string;
    priority: number;
    assigned_to: string | null;
    created_by: string;
  }) => void;
  onCancel: () => void;
}

export function TaskForm({ projectId, agents, onSubmit, onCancel }: Props) {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<TaskPriority>(3);
  const [assignedTo, setAssignedTo] = useState("");
  const [createdBy, setCreatedBy] = useState("");

  // suppress unused variable warning — projectId is used for context
  void projectId;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim() || !createdBy.trim()) return;
    const id = `task-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    onSubmit({
      id,
      title: title.trim(),
      description: description.trim(),
      priority,
      assigned_to: assignedTo || null,
      created_by: createdBy.trim(),
    });
  }

  return (
    <form onSubmit={handleSubmit} className="px-4 py-3 bg-gray-900/80 border-b border-gray-800 space-y-2">
      <input
        type="text"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="Task title"
        autoFocus
        className="w-full bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none placeholder-gray-500"
      />
      <textarea
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        placeholder="Description (optional)"
        className="w-full bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none resize-none placeholder-gray-500"
        rows={2}
      />
      <div className="grid grid-cols-2 gap-2">
        <select
          value={priority}
          onChange={(e) => setPriority(Number(e.target.value) as TaskPriority)}
          className="bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none"
        >
          {PRIORITY_OPTIONS.map((p) => (
            <option key={p.value} value={p.value}>{p.label}</option>
          ))}
        </select>
        <select
          value={assignedTo}
          onChange={(e) => setAssignedTo(e.target.value)}
          className="bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none"
        >
          <option value="">Unassigned</option>
          {agents.map((a) => (
            <option key={a.name} value={a.name}>{a.name}</option>
          ))}
        </select>
      </div>
      <input
        type="text"
        value={createdBy}
        onChange={(e) => setCreatedBy(e.target.value)}
        placeholder="Created by (agent name)"
        className="w-full bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none placeholder-gray-500"
      />
      <div className="flex gap-2 justify-end">
        <button
          type="button"
          onClick={onCancel}
          className="text-xs px-3 py-1.5 rounded-md text-gray-400 hover:text-gray-200 bg-gray-800 hover:bg-gray-700 transition-colors"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={!title.trim() || !createdBy.trim()}
          className="text-xs px-3 py-1.5 rounded-md font-medium bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          Create Task
        </button>
      </div>
    </form>
  );
}

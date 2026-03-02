import { useState, useEffect } from "react";
import type { TaskComment, TaskStatus, TaskPriority, Agent } from "../types";
import { useTaskDetail } from "../hooks/useTasks";

const STATUS_OPTIONS: { value: TaskStatus; label: string }[] = [
  { value: "backlog", label: "Backlog" },
  { value: "todo", label: "Todo" },
  { value: "in_progress", label: "In Progress" },
  { value: "review", label: "Review" },
  { value: "done", label: "Done" },
];

const PRIORITY_LABELS: Record<TaskPriority, { label: string; color: string }> = {
  1: { label: "Critical", color: "text-red-400" },
  2: { label: "High", color: "text-orange-400" },
  3: { label: "Medium", color: "text-blue-400" },
  4: { label: "Low", color: "text-gray-400" },
};

function formatDateTime(ts: string): string {
  return new Date(ts).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    backlog: "bg-gray-500/20 text-gray-400",
    todo: "bg-blue-500/20 text-blue-400",
    in_progress: "bg-amber-500/20 text-amber-400",
    review: "bg-purple-500/20 text-purple-400",
    done: "bg-green-500/20 text-green-400",
  };
  return (
    <span className={`${colors[status] ?? "bg-gray-500/20 text-gray-400"} text-xs px-2 py-0.5 rounded-full font-medium`}>
      {status.replace(/_/g, " ")}
    </span>
  );
}

function CommentItem({ comment }: { comment: TaskComment }) {
  return (
    <div className="flex gap-3 py-2">
      <div className="w-6 h-6 rounded-full bg-gray-600 flex items-center justify-center text-[10px] font-bold text-white shrink-0 mt-0.5">
        {comment.agent_name.charAt(0).toUpperCase()}
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-gray-200">{comment.agent_name}</span>
          <span className="text-xs text-gray-500">{formatDateTime(comment.created_at)}</span>
        </div>
        <p className="text-sm text-gray-300 mt-0.5 whitespace-pre-wrap break-words">{comment.content}</p>
      </div>
    </div>
  );
}

interface Props {
  taskId: string;
  agents: Agent[];
  onClose: () => void;
  onUpdate: () => void;
}

export function TaskDetail({ taskId, agents, onClose, onUpdate }: Props) {
  const { task, comments, dependencies, loading, refetch } = useTaskDetail(taskId);
  const [commentText, setCommentText] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // Close on Escape
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [onClose]);

  async function handleStatusChange(status: string) {
    if (!task) return;
    try {
      await fetch(`/api/tasks/${task.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status }),
      });
      void refetch();
      onUpdate();
    } catch {
      // ignore
    }
  }

  async function handleAssigneeChange(assignedTo: string) {
    if (!task) return;
    try {
      await fetch(`/api/tasks/${task.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ assigned_to: assignedTo || null }),
      });
      void refetch();
      onUpdate();
    } catch {
      // ignore
    }
  }

  async function handleAddComment(e: React.FormEvent) {
    e.preventDefault();
    if (!task || !commentText.trim()) return;
    setSubmitting(true);
    try {
      await fetch(`/api/tasks/${task.id}/comments`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent_name: "pixel", content: commentText.trim() }),
      });
      setCommentText("");
      void refetch();
    } catch {
      // ignore
    } finally {
      setSubmitting(false);
    }
  }

  if (loading && !task) {
    return (
      <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
        <div className="bg-gray-900 rounded-xl p-6">
          <p className="text-sm text-gray-400">Loading…</p>
        </div>
      </div>
    );
  }

  if (!task) return null;

  const priorityInfo = PRIORITY_LABELS[task.priority as TaskPriority] ?? PRIORITY_LABELS[3];
  const undoneDeps = dependencies.filter((d) => d.status !== "done");

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4" onClick={onClose}>
      <div
        className="bg-gray-900 rounded-xl border border-gray-700 w-full max-w-2xl max-h-[85vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="px-5 py-4 border-b border-gray-800 flex items-start justify-between gap-3 shrink-0">
          <div className="min-w-0">
            <h2 className="text-lg font-semibold text-white">{task.title}</h2>
            <div className="flex items-center gap-2 mt-1">
              <span className={`text-xs font-medium ${priorityInfo.color}`}>{priorityInfo.label}</span>
              <span className="text-xs text-gray-500">·</span>
              <span className="text-xs text-gray-500">{task.id}</span>
            </div>
          </div>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-300 transition-colors text-lg shrink-0"
          >
            ✕
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
          {/* Description */}
          {task.description && (
            <div>
              <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-1">Description</h3>
              <p className="text-sm text-gray-300 whitespace-pre-wrap">{task.description}</p>
            </div>
          )}

          {/* Fields */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-1">Status</h3>
              <select
                value={task.status}
                onChange={(e) => void handleStatusChange(e.target.value)}
                className="w-full bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none"
              >
                {STATUS_OPTIONS.map((s) => (
                  <option key={s.value} value={s.value}>{s.label}</option>
                ))}
              </select>
            </div>
            <div>
              <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-1">Assignee</h3>
              <select
                value={task.assigned_to ?? ""}
                onChange={(e) => void handleAssigneeChange(e.target.value)}
                className="w-full bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none"
              >
                <option value="">Unassigned</option>
                {agents.map((a) => (
                  <option key={a.name} value={a.name}>{a.name}</option>
                ))}
              </select>
            </div>
          </div>

          {/* Meta */}
          <div className="flex items-center gap-4 text-xs text-gray-500">
            <span>Created by {task.created_by}</span>
            <span>·</span>
            <span>{formatDateTime(task.created_at)}</span>
          </div>

          {/* Dependencies */}
          {dependencies.length > 0 && (
            <div>
              <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2">
                Dependencies ({dependencies.length})
              </h3>
              <div className="space-y-1.5">
                {dependencies.map((dep) => (
                  <div
                    key={dep.id}
                    className="flex items-center gap-2 px-3 py-2 bg-gray-800/50 rounded-lg border border-gray-700/50"
                  >
                    <StatusBadge status={dep.status} />
                    <span className="text-sm text-gray-300 truncate">{dep.title}</span>
                  </div>
                ))}
              </div>
              {undoneDeps.length > 0 && (
                <p className="text-xs text-red-400 mt-1.5">
                  ⚠ {undoneDeps.length} blocking {undoneDeps.length === 1 ? "task" : "tasks"} not done
                </p>
              )}
            </div>
          )}

          {/* Comments */}
          <div>
            <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2">
              Comments ({comments.length})
            </h3>
            {comments.length === 0 && (
              <p className="text-sm text-gray-600">No comments yet</p>
            )}
            <div className="space-y-1 divide-y divide-gray-800/50">
              {comments.map((c) => (
                <CommentItem key={c.id} comment={c} />
              ))}
            </div>

            {/* Add comment form */}
            <form onSubmit={(e) => void handleAddComment(e)} className="mt-3 flex gap-2">
              <input
                type="text"
                value={commentText}
                onChange={(e) => setCommentText(e.target.value)}
                placeholder="Add a comment…"
                className="flex-1 bg-gray-800 text-sm text-gray-200 rounded px-3 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none placeholder-gray-500"
              />
              <button
                type="submit"
                disabled={!commentText.trim() || submitting}
                className="text-xs px-3 py-1.5 rounded-md font-medium bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                Post
              </button>
            </form>
          </div>
        </div>
      </div>
    </div>
  );
}

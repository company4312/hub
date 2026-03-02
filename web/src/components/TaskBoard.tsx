import { useState, useMemo, useEffect, useCallback } from "react";
import type { Agent, Task, TaskStatus } from "../types";
import { useProjects, useTasks } from "../hooks/useTasks";
import { TaskCard } from "./TaskCard";
import { TaskDetail } from "./TaskDetail";
import { ProjectForm } from "./ProjectForm";
import { TaskForm } from "./TaskForm";

const COLUMNS: { status: TaskStatus; label: string; color: string }[] = [
  { status: "backlog", label: "Backlog", color: "border-gray-500" },
  { status: "todo", label: "Todo", color: "border-blue-500" },
  { status: "in_progress", label: "In Progress", color: "border-amber-500" },
  { status: "review", label: "Review", color: "border-purple-500" },
  { status: "done", label: "Done", color: "border-green-500" },
];

interface Props {
  agents: Agent[];
}

export function TaskBoard({ agents }: Props) {
  const { projects, refetch: refetchProjects } = useProjects();
  const [selectedProjectId, setSelectedProjectId] = useState<string>("");
  const { tasks, loading, refetch: refetchTasks } = useTasks(selectedProjectId || undefined);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [showProjectForm, setShowProjectForm] = useState(false);
  const [showTaskForm, setShowTaskForm] = useState(false);

  // Track blocking counts per task
  const [blockingCounts, setBlockingCounts] = useState<Record<string, number>>({});

  // Auto-select first project
  useEffect(() => {
    if (!selectedProjectId && projects.length > 0) {
      setSelectedProjectId(projects[0]!.id);
    }
  }, [projects, selectedProjectId]);

  // Fetch blocking counts for all tasks
  const fetchBlockingCounts = useCallback(async () => {
    const counts: Record<string, number> = {};
    await Promise.all(
      tasks.map(async (t) => {
        try {
          const res = await fetch(`/api/tasks/${t.id}/dependencies`);
          if (res.ok) {
            const deps = (await res.json()) as Task[];
            const undone = deps.filter((d) => d.status !== "done").length;
            counts[t.id] = undone;
          }
        } catch {
          // ignore
        }
      })
    );
    setBlockingCounts(counts);
  }, [tasks]);

  useEffect(() => {
    if (tasks.length > 0) {
      void fetchBlockingCounts();
    } else {
      setBlockingCounts({});
    }
  }, [tasks, fetchBlockingCounts]);

  const grouped = useMemo(() => {
    const groups: Record<TaskStatus, Task[]> = {
      backlog: [],
      todo: [],
      in_progress: [],
      review: [],
      done: [],
    };
    for (const t of tasks) {
      const col = groups[t.status as TaskStatus];
      if (col) col.push(t);
    }
    return groups;
  }, [tasks]);

  async function handleCreateProject(data: { id: string; name: string; description: string; created_by: string }) {
    try {
      const res = await fetch("/api/projects", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setShowProjectForm(false);
      await refetchProjects();
      setSelectedProjectId(data.id);
    } catch {
      // keep form open
    }
  }

  async function handleCreateTask(data: {
    id: string;
    title: string;
    description: string;
    priority: number;
    assigned_to: string | null;
    created_by: string;
  }) {
    if (!selectedProjectId) return;
    try {
      const res = await fetch(`/api/projects/${selectedProjectId}/tasks`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setShowTaskForm(false);
      void refetchTasks();
    } catch {
      // keep form open
    }
  }

  const selectedProject = projects.find((p) => p.id === selectedProjectId);

  return (
    <div className="flex-1 flex flex-col min-w-0">
      {/* Header */}
      <header className="px-4 py-3 border-b border-gray-800 bg-gray-900/80 backdrop-blur-sm flex items-center gap-3 shrink-0">
        <select
          value={selectedProjectId}
          onChange={(e) => {
            setSelectedProjectId(e.target.value);
            setShowTaskForm(false);
          }}
          className="bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none"
        >
          <option value="">Select project…</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>{p.name}</option>
          ))}
        </select>

        <button
          onClick={() => { setShowProjectForm((v) => !v); setShowTaskForm(false); }}
          className="text-xs px-3 py-1.5 rounded-md font-medium bg-gray-700 text-gray-200 hover:bg-gray-600 transition-colors"
        >
          {showProjectForm ? "Cancel" : "+ New Project"}
        </button>

        {selectedProjectId && (
          <button
            onClick={() => { setShowTaskForm((v) => !v); setShowProjectForm(false); }}
            className="text-xs px-3 py-1.5 rounded-md font-medium bg-blue-600 text-white hover:bg-blue-500 transition-colors"
          >
            {showTaskForm ? "Cancel" : "+ New Task"}
          </button>
        )}

        {selectedProject && (
          <span className="text-xs text-gray-500 ml-auto">
            {tasks.length} {tasks.length === 1 ? "task" : "tasks"}
          </span>
        )}
      </header>

      {/* Project form */}
      {showProjectForm && (
        <ProjectForm
          onSubmit={(data) => void handleCreateProject(data)}
          onCancel={() => setShowProjectForm(false)}
        />
      )}

      {/* Task form */}
      {showTaskForm && selectedProjectId && (
        <TaskForm
          projectId={selectedProjectId}
          agents={agents}
          onSubmit={(data) => void handleCreateTask(data)}
          onCancel={() => setShowTaskForm(false)}
        />
      )}

      {/* Board */}
      {!selectedProjectId && !showProjectForm && (
        <div className="flex-1 flex items-center justify-center">
          <p className="text-sm text-gray-600">Select or create a project to get started</p>
        </div>
      )}

      {selectedProjectId && (
        <div className="flex-1 overflow-x-auto overflow-y-hidden">
          <div className="flex gap-3 p-4 h-full min-w-min">
            {COLUMNS.map((col) => {
              const columnTasks = grouped[col.status];
              return (
                <div key={col.status} className="w-60 shrink-0 flex flex-col">
                  <div className={`flex items-center gap-2 mb-3 pb-2 border-b-2 ${col.color}`}>
                    <h3 className="text-sm font-medium text-gray-300">{col.label}</h3>
                    <span className="text-xs text-gray-500 bg-gray-800 px-1.5 py-0.5 rounded-full">
                      {columnTasks.length}
                    </span>
                  </div>
                  <div className="flex-1 overflow-y-auto space-y-2">
                    {columnTasks.map((t) => (
                      <TaskCard
                        key={t.id}
                        task={t}
                        blockingCount={blockingCounts[t.id] ?? 0}
                        onClick={() => setSelectedTaskId(t.id)}
                      />
                    ))}
                    {columnTasks.length === 0 && (
                      <p className="text-xs text-gray-600 text-center py-4">No tasks</p>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {loading && (
        <div className="absolute inset-0 flex items-center justify-center bg-gray-950/50 pointer-events-none">
          <p className="text-sm text-gray-400">Loading tasks…</p>
        </div>
      )}

      {/* Task detail modal */}
      {selectedTaskId && (
        <TaskDetail
          taskId={selectedTaskId}
          agents={agents}
          onClose={() => setSelectedTaskId(null)}
          onUpdate={() => void refetchTasks()}
        />
      )}
    </div>
  );
}

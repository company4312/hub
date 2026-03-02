import type { Task, TaskStatus, TaskPriority } from "../types";

const PRIORITY_COLORS: Record<TaskPriority, { bg: string; text: string; label: string }> = {
  1: { bg: "bg-red-500/20", text: "text-red-400", label: "Critical" },
  2: { bg: "bg-orange-500/20", text: "text-orange-400", label: "High" },
  3: { bg: "bg-blue-500/20", text: "text-blue-400", label: "Medium" },
  4: { bg: "bg-gray-500/20", text: "text-gray-400", label: "Low" },
};

const AGENT_COLORS = [
  "bg-blue-500",
  "bg-green-500",
  "bg-purple-500",
  "bg-amber-500",
  "bg-rose-500",
  "bg-cyan-500",
  "bg-indigo-500",
  "bg-emerald-500",
];

function agentColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash);
  }
  return AGENT_COLORS[Math.abs(hash) % AGENT_COLORS.length]!;
}

interface Props {
  task: Task;
  blockingCount: number;
  onClick: (task: Task) => void;
}

export function TaskCard({ task, blockingCount, onClick }: Props) {
  const priority = PRIORITY_COLORS[task.priority as TaskPriority] ?? PRIORITY_COLORS[3];
  const hasBlockers = blockingCount > 0 && task.status !== ("done" as TaskStatus);

  return (
    <div
      onClick={() => onClick(task)}
      className="px-3 py-2.5 bg-gray-800/50 rounded-lg border border-gray-700/50 hover:border-gray-600/50 transition-colors cursor-pointer group"
    >
      <p className="text-sm text-gray-200 font-medium truncate group-hover:text-white transition-colors">
        {task.title}
      </p>
      <div className="flex items-center gap-2 mt-2 flex-wrap">
        <span className={`${priority.bg} ${priority.text} text-xs px-1.5 py-0.5 rounded font-medium`}>
          {priority.label}
        </span>
        {task.assigned_to && (
          <div className="flex items-center gap-1">
            <div
              className={`${agentColor(task.assigned_to)} w-4 h-4 rounded-full flex items-center justify-center text-[9px] font-bold text-white`}
            >
              {task.assigned_to.charAt(0).toUpperCase()}
            </div>
            <span className="text-xs text-gray-400">{task.assigned_to}</span>
          </div>
        )}
        {hasBlockers && (
          <span className="text-xs text-red-400 bg-red-500/10 px-1.5 py-0.5 rounded">
            Blocked by {blockingCount}
          </span>
        )}
      </div>
    </div>
  );
}

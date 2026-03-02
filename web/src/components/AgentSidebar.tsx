import type { Agent } from "../types";

interface Props {
  agents: Agent[];
  selected: string | null;
  onSelect: (name: string | null) => void;
  loading: boolean;
  error: string | null;
}

export function AgentSidebar({ agents, selected, onSelect, loading, error }: Props) {
  return (
    <aside className="w-64 bg-gray-900 border-r border-gray-800 flex flex-col shrink-0">
      <div className="px-4 py-4 border-b border-gray-800">
        <h1 className="text-lg font-bold text-white tracking-tight">Company4312</h1>
        <p className="text-xs text-gray-500 mt-0.5">Agent Dashboard</p>
      </div>

      <nav className="flex-1 overflow-y-auto py-2">
        <button
          onClick={() => onSelect(null)}
          className={`w-full text-left px-4 py-2.5 text-sm transition-colors ${
            selected === null
              ? "bg-gray-800 text-white font-medium"
              : "text-gray-400 hover:text-gray-200 hover:bg-gray-800/50"
          }`}
        >
          All Activity
        </button>

        {loading && (
          <p className="px-4 py-3 text-xs text-gray-500">Loading agents…</p>
        )}

        {error && (
          <p className="px-4 py-3 text-xs text-red-400">Error: {error}</p>
        )}

        {agents.map((agent) => (
          <button
            key={agent.name}
            onClick={() => onSelect(agent.name)}
            className={`w-full text-left px-4 py-2.5 transition-colors ${
              selected === agent.name
                ? "bg-gray-800 text-white"
                : "text-gray-400 hover:text-gray-200 hover:bg-gray-800/50"
            }`}
          >
            <span className="text-sm font-medium">{agent.name}</span>
            <span className="block text-xs text-gray-500 mt-0.5">{agent.title}</span>
          </button>
        ))}
      </nav>
    </aside>
  );
}

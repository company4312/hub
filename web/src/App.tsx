import { useState } from "react";
import { useAgents } from "./hooks/useAgents";
import { useActivityStream } from "./hooks/useActivityStream";
import { AgentSidebar } from "./components/AgentSidebar";
import { ActivityFeed } from "./components/ActivityFeed";
import { MemoryPanel } from "./components/MemoryPanel";
import { TaskBoard } from "./components/TaskBoard";

type Tab = "activity" | "memories" | "tasks";

export default function App() {
  const [selectedAgent, setSelectedAgent] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<Tab>("activity");
  const { agents, loading, error } = useAgents();
  const { entries, connected } = useActivityStream(selectedAgent ?? undefined);

  const agentTitle = agents.find((a) => a.name === selectedAgent)?.title;

  return (
    <div className="flex h-screen bg-gray-950 text-gray-100">
      <AgentSidebar
        agents={agents}
        selected={selectedAgent}
        onSelect={(name) => {
          setSelectedAgent(name);
          if (!name) setActiveTab("activity");
        }}
        loading={loading}
        error={error}
      />

      <div className="flex-1 flex flex-col min-w-0">
        {/* Tabs */}
        <div className="flex border-b border-gray-800 bg-gray-900/80 shrink-0">
          <button
            onClick={() => setActiveTab("activity")}
            className={`px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === "activity"
                ? "text-white border-b-2 border-blue-500"
                : "text-gray-400 hover:text-gray-200"
            }`}
          >
            Activity
          </button>
          {selectedAgent && (
            <button
              onClick={() => setActiveTab("memories")}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === "memories"
                  ? "text-white border-b-2 border-blue-500"
                  : "text-gray-400 hover:text-gray-200"
              }`}
            >
              Memories
            </button>
          )}
          <button
            onClick={() => setActiveTab("tasks")}
            className={`px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === "tasks"
                ? "text-white border-b-2 border-blue-500"
                : "text-gray-400 hover:text-gray-200"
            }`}
          >
            Tasks
          </button>
        </div>

        {/* Content */}
        {activeTab === "tasks" ? (
          <TaskBoard agents={agents} />
        ) : activeTab === "memories" && selectedAgent ? (
          <MemoryPanel agentName={selectedAgent} />
        ) : (
          <ActivityFeed
            entries={entries}
            connected={connected}
            selectedAgent={selectedAgent}
            agentTitle={agentTitle}
          />
        )}
      </div>
    </div>
  );
}

import { useState } from "react";
import { useAgents } from "./hooks/useAgents";
import { useActivityStream } from "./hooks/useActivityStream";
import { AgentSidebar } from "./components/AgentSidebar";
import { ActivityFeed } from "./components/ActivityFeed";

export default function App() {
  const [selectedAgent, setSelectedAgent] = useState<string | null>(null);
  const { agents, loading, error } = useAgents();
  const { entries, connected } = useActivityStream(selectedAgent ?? undefined);

  const agentTitle = agents.find((a) => a.name === selectedAgent)?.title;

  return (
    <div className="flex h-screen bg-gray-950 text-gray-100">
      <AgentSidebar
        agents={agents}
        selected={selectedAgent}
        onSelect={setSelectedAgent}
        loading={loading}
        error={error}
      />
      <ActivityFeed
        entries={entries}
        connected={connected}
        selectedAgent={selectedAgent}
        agentTitle={agentTitle}
      />
    </div>
  );
}

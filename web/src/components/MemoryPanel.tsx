import { useState, useMemo } from "react";
import { useMemories } from "../hooks/useMemories";
import type { Memory } from "../types";

const CATEGORY_COLORS: Record<string, { bg: string; text: string }> = {
  lesson_learned: { bg: "bg-amber-500/20", text: "text-amber-400" },
  preference: { bg: "bg-blue-500/20", text: "text-blue-400" },
  context: { bg: "bg-green-500/20", text: "text-green-400" },
  decision: { bg: "bg-purple-500/20", text: "text-purple-400" },
  skill: { bg: "bg-cyan-500/20", text: "text-cyan-400" },
};

const DEFAULT_CATEGORY_COLOR = { bg: "bg-gray-500/20", text: "text-gray-400" };

const CATEGORIES = ["lesson_learned", "preference", "context", "decision", "skill", "other"];

function categoryColor(cat: string) {
  return CATEGORY_COLORS[cat] ?? DEFAULT_CATEGORY_COLOR;
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function MemoryCard({
  memory,
  onDelete,
  onUpdate,
}: {
  memory: Memory;
  onDelete: (id: number) => void;
  onUpdate: (id: number, content: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [editContent, setEditContent] = useState(memory.content);
  const colors = categoryColor(memory.category);

  function handleSave() {
    if (editContent.trim() && editContent !== memory.content) {
      onUpdate(memory.id, editContent.trim());
    }
    setEditing(false);
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSave();
    }
    if (e.key === "Escape") {
      setEditContent(memory.content);
      setEditing(false);
    }
  }

  return (
    <div className="group px-3 py-2.5 bg-gray-800/50 rounded-lg border border-gray-700/50 hover:border-gray-600/50 transition-colors">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          {editing ? (
            <textarea
              value={editContent}
              onChange={(e) => setEditContent(e.target.value)}
              onBlur={handleSave}
              onKeyDown={handleKeyDown}
              autoFocus
              className="w-full bg-gray-900 text-sm text-gray-200 rounded px-2 py-1 border border-gray-600 focus:border-blue-500 focus:outline-none resize-none"
              rows={2}
            />
          ) : (
            <p
              onClick={() => {
                setEditing(true);
                setEditContent(memory.content);
              }}
              className="text-sm text-gray-200 cursor-pointer hover:text-white transition-colors"
            >
              {memory.content}
            </p>
          )}
          <div className="flex items-center gap-2 mt-1.5">
            <span className={`${colors.bg} ${colors.text} text-xs px-1.5 py-0.5 rounded font-medium`}>
              {memory.category.replace(/_/g, " ")}
            </span>
            {memory.source && (
              <span className="text-xs text-gray-500 truncate">{memory.source}</span>
            )}
            <span className="text-xs text-gray-600 ml-auto shrink-0">
              {formatDate(memory.created_at)}
            </span>
          </div>
        </div>
        <button
          onClick={() => onDelete(memory.id)}
          className="text-gray-600 hover:text-red-400 text-sm opacity-0 group-hover:opacity-100 transition-opacity shrink-0 mt-0.5"
          title="Delete memory"
        >
          ✕
        </button>
      </div>
    </div>
  );
}

function AddMemoryForm({
  agentName,
  onAdd,
  onCancel,
}: {
  agentName: string;
  onAdd: (data: { agent_name: string; category: string; content: string; source: string }) => void;
  onCancel: () => void;
}) {
  const [category, setCategory] = useState("context");
  const [content, setContent] = useState("");
  const [source, setSource] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!content.trim()) return;
    onAdd({ agent_name: agentName, category, content: content.trim(), source: source.trim() });
    setContent("");
    setSource("");
  }

  return (
    <form onSubmit={handleSubmit} className="px-4 py-3 bg-gray-900/80 border-b border-gray-800 space-y-2">
      <select
        value={category}
        onChange={(e) => setCategory(e.target.value)}
        className="w-full bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none"
      >
        {CATEGORIES.map((c) => (
          <option key={c} value={c}>
            {c.replace(/_/g, " ")}
          </option>
        ))}
      </select>
      <textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="Memory content…"
        className="w-full bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none resize-none"
        rows={2}
      />
      <input
        type="text"
        value={source}
        onChange={(e) => setSource(e.target.value)}
        placeholder="Source (optional)"
        className="w-full bg-gray-800 text-sm text-gray-200 rounded px-2 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none"
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
          disabled={!content.trim()}
          className="text-xs px-3 py-1.5 rounded-md font-medium bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          Add Memory
        </button>
      </div>
    </form>
  );
}

interface Props {
  agentName: string;
}

export function MemoryPanel({ agentName }: Props) {
  const { memories, loading, error, refetch } = useMemories(agentName);
  const [showForm, setShowForm] = useState(false);
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    if (!search.trim()) return memories;
    const q = search.toLowerCase();
    return memories.filter((m) => m.content.toLowerCase().includes(q));
  }, [memories, search]);

  const grouped = useMemo(() => {
    const groups: Record<string, Memory[]> = {};
    for (const m of filtered) {
      (groups[m.category] ??= []).push(m);
    }
    return groups;
  }, [filtered]);

  async function handleAdd(data: { agent_name: string; category: string; content: string; source: string }) {
    try {
      const res = await fetch("/api/memories", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setShowForm(false);
      void refetch();
    } catch {
      // keep form open on error
    }
  }

  async function handleDelete(id: number) {
    try {
      await fetch(`/api/memories/${id}`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent_name: agentName }),
      });
      void refetch();
    } catch {
      // ignore
    }
  }

  async function handleUpdate(id: number, content: string) {
    try {
      await fetch(`/api/memories/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent_name: agentName, content }),
      });
      void refetch();
    } catch {
      // ignore
    }
  }

  return (
    <div className="flex-1 flex flex-col min-w-0">
      {/* Header */}
      <header className="px-4 py-3 border-b border-gray-800 bg-gray-900/80 backdrop-blur-sm flex items-center justify-between shrink-0">
        <div>
          <h2 className="text-base font-semibold text-white">{agentName} — Memories</h2>
          <p className="text-xs text-gray-500 mt-0.5">{memories.length} memories</p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="text-xs px-3 py-1.5 rounded-md font-medium bg-blue-600 text-white hover:bg-blue-500 transition-colors"
        >
          {showForm ? "Cancel" : "+ Add Memory"}
        </button>
      </header>

      {/* Add form */}
      {showForm && (
        <AddMemoryForm agentName={agentName} onAdd={handleAdd} onCancel={() => setShowForm(false)} />
      )}

      {/* Search */}
      <div className="px-4 py-2 border-b border-gray-800">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search memories…"
          className="w-full bg-gray-800 text-sm text-gray-200 rounded px-3 py-1.5 border border-gray-700 focus:border-blue-500 focus:outline-none placeholder-gray-500"
        />
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-4 py-3 space-y-4">
        {loading && (
          <p className="text-sm text-gray-500 text-center py-8">Loading memories…</p>
        )}
        {error && (
          <p className="text-sm text-red-400 text-center py-8">Error: {error}</p>
        )}
        {!loading && !error && filtered.length === 0 && (
          <p className="text-sm text-gray-600 text-center py-8">
            {search ? "No memories match your search" : "No memories yet"}
          </p>
        )}
        {Object.entries(grouped).map(([category, items]) => {
          const colors = categoryColor(category);
          return (
            <div key={category}>
              <h3 className="flex items-center gap-2 mb-2">
                <span className={`${colors.bg} ${colors.text} text-xs px-2 py-0.5 rounded-full font-medium`}>
                  {category.replace(/_/g, " ")}
                </span>
                <span className="text-xs text-gray-600">{items.length}</span>
              </h3>
              <div className="space-y-1.5">
                {items.map((m) => (
                  <MemoryCard key={m.id} memory={m} onDelete={handleDelete} onUpdate={handleUpdate} />
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

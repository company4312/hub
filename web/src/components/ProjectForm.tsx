import { useState } from "react";

interface Props {
  onSubmit: (data: { id: string; name: string; description: string; created_by: string }) => void;
  onCancel: () => void;
}

export function ProjectForm({ onSubmit, onCancel }: Props) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [createdBy, setCreatedBy] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !createdBy.trim()) return;
    const id = name.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
    onSubmit({ id, name: name.trim(), description: description.trim(), created_by: createdBy.trim() });
  }

  return (
    <form onSubmit={handleSubmit} className="px-4 py-3 bg-gray-900/80 border-b border-gray-800 space-y-2">
      <input
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Project name"
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
          disabled={!name.trim() || !createdBy.trim()}
          className="text-xs px-3 py-1.5 rounded-md font-medium bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          Create Project
        </button>
      </div>
    </form>
  );
}

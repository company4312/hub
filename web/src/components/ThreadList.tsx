import type { Thread } from '../types';

function hashColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash);
  const colors = ['#3b82f6', '#8b5cf6', '#10b981', '#f59e0b', '#ef4444', '#06b6d4', '#ec4899'];
  return colors[Math.abs(hash) % colors.length]!;
}

function timeAgo(ts: string): string {
  const diff = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

interface Props {
  threads: Thread[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}

export function ThreadList({ threads, selectedId, onSelect }: Props) {
  return (
    <div className="w-80 border-r border-gray-800 flex flex-col h-full">
      <div className="p-4 border-b border-gray-800">
        <h1 className="text-lg font-semibold text-gray-100">Threads</h1>
        <p className="text-xs text-gray-500 mt-1">{threads.length} conversation{threads.length !== 1 ? 's' : ''}</p>
      </div>
      <div className="flex-1 overflow-y-auto">
        {threads.length === 0 && (
          <p className="text-gray-500 text-sm p-4">No threads yet. Send a message via Telegram to start one.</p>
        )}
        {threads.map(t => {
          const isSelected = t.id === selectedId;
          const preview = t.last_message?.content || '';
          const previewText = preview.length > 80 ? preview.slice(0, 80) + '…' : preview;
          const fromName = t.last_message?.from_name;
          return (
            <button
              key={t.id}
              onClick={() => onSelect(t.id)}
              className={`w-full text-left p-3 border-b border-gray-800/50 hover:bg-gray-800/50 transition-colors ${
                isSelected ? 'bg-gray-800' : ''
              }`}
            >
              <div className="flex items-center justify-between mb-1">
                <span className="text-sm font-medium text-gray-200 truncate flex-1">{t.title}</span>
                <span className="text-xs text-gray-500 ml-2 flex-shrink-0">{timeAgo(t.updated_at)}</span>
              </div>
              {t.last_message && (
                <div className="flex items-center gap-1.5">
                  {fromName && (
                    <span
                      className="text-xs font-medium flex-shrink-0"
                      style={{ color: hashColor(fromName) }}
                    >
                      {fromName}:
                    </span>
                  )}
                  <span className="text-xs text-gray-400 truncate">{previewText}</span>
                </div>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

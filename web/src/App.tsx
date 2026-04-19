import { useState } from 'react';
import { useThreads } from './hooks/useThreads';
import { useMessages } from './hooks/useMessages';
import { ThreadList } from './components/ThreadList';
import { ThreadView } from './components/ThreadView';

export default function App() {
  const { threads, loading: threadsLoading } = useThreads();
  const [selectedThreadId, setSelectedThreadId] = useState<string | null>(null);
  const { messages, loading: messagesLoading } = useMessages(selectedThreadId);

  const selectedThread = threads.find(t => t.id === selectedThreadId);

  return (
    <div className="h-screen flex bg-gray-950 text-gray-100">
      <ThreadList
        threads={threads}
        selectedId={selectedThreadId}
        onSelect={setSelectedThreadId}
      />
      <div className="flex-1 flex flex-col">
        {selectedThread ? (
          <ThreadView
            messages={messages}
            threadTitle={selectedThread.title}
            loading={messagesLoading}
          />
        ) : (
          <div className="flex-1 flex items-center justify-center">
            <div className="text-center text-gray-500">
              {threadsLoading ? (
                <p>Loading…</p>
              ) : threads.length === 0 ? (
                <div>
                  <p className="text-lg mb-2">No threads yet</p>
                  <p className="text-sm">Send a message via Telegram to start a conversation.</p>
                </div>
              ) : (
                <p className="text-lg">Select a thread to view messages</p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

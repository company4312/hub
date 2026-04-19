package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestNew_CreatesTables(t *testing.T) {
	s := newTestStore(t)
	tables := []string{"agents", "memories", "projects", "tasks", "task_dependencies", "task_comments", "threads", "messages", "session_events", "thread_sessions", "schema_migrations"}
	for _, tbl := range tables {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n); err != nil {
			t.Errorf("table %s should exist: %v", tbl, err)
		}
	}
}

func TestNew_Idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s1, _ := New(dbPath)
	_ = s1.Close()
	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	_ = s2.Close()
}

func TestRegisterAgent_And_GetAgent(t *testing.T) {
	s := newTestStore(t)
	_ = s.RegisterAgent(AgentConfig{Name: "atlas", Title: "Atlas", SystemPrompt: "You are Atlas.", Model: "gpt-4"})
	got, _ := s.GetAgent("atlas")
	if got == nil || got.Name != "atlas" || got.Title != "Atlas" {
		t.Errorf("unexpected agent: %+v", got)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, _ := s.GetAgent("nonexistent")
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestRegisterAgent_Update(t *testing.T) {
	s := newTestStore(t)
	_ = s.RegisterAgent(AgentConfig{Name: "bot", Title: "Bot", SystemPrompt: "v1", Model: "m1"})
	_ = s.RegisterAgent(AgentConfig{Name: "bot", Title: "Bot v2", SystemPrompt: "v2", Model: "m2"})
	got, _ := s.GetAgent("bot")
	if got.SystemPrompt != "v2" || got.Title != "Bot v2" {
		t.Errorf("agent not updated: %+v", got)
	}
}

func TestListAgents(t *testing.T) {
	s := newTestStore(t)
	for _, name := range []string{"charlie", "alice", "bob"} {
		_ = s.RegisterAgent(AgentConfig{Name: name, Title: name, SystemPrompt: "p", Model: "m"})
	}
	agents, _ := s.ListAgents()
	if len(agents) != 3 || agents[0].Name != "alice" {
		t.Errorf("unexpected agents: %+v", agents)
	}
}

func TestCreateThread_And_GetThread(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 42, Title: "Test thread", Status: "active"})
	got, _ := s.GetThread("t1")
	if got == nil || got.ChatID != 42 || got.Title != "Test thread" {
		t.Errorf("unexpected thread: %+v", got)
	}
}

func TestGetThread_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, _ := s.GetThread("nope")
	if got != nil {
		t.Errorf("expected nil")
	}
}

func TestListThreads(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "First", Status: "active"})
	_ = s.CreateThread(Thread{ID: "t2", ChatID: 1, Title: "Second", Status: "active"})
	_ = s.CreateThread(Thread{ID: "t3", ChatID: 2, Title: "Other chat", Status: "active"})

	all, _ := s.ListThreads(ThreadFilter{})
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	chat1, _ := s.ListThreads(ThreadFilter{ChatID: 1})
	if len(chat1) != 2 {
		t.Errorf("expected 2, got %d", len(chat1))
	}
}

func TestUpdateThreadStatus(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	_ = s.UpdateThreadStatus("t1", "closed")
	got, _ := s.GetThread("t1")
	if got.Status != "closed" {
		t.Errorf("expected closed, got %s", got.Status)
	}
}

func TestUpdateThreadStatus_NotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpdateThreadStatus("nope", "closed"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAddMessage_And_ListMessages(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	id, _ := s.AddMessage(Message{ThreadID: "t1", FromName: "user", Content: "hello", MessageType: "user_message"})
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}
	_, _ = s.AddMessage(Message{ThreadID: "t1", FromName: "cto", ToName: "atlas", Content: "handle this", MessageType: "agent_message", CopilotSessionID: "sess-1"})

	msgs, _ := s.ListMessages("t1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2, got %d", len(msgs))
	}
	if msgs[0].FromName != "user" || msgs[1].ToName != "atlas" {
		t.Errorf("unexpected messages: %+v, %+v", msgs[0], msgs[1])
	}
}

func TestGetMessage(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	id, _ := s.AddMessage(Message{ThreadID: "t1", FromName: "user", Content: "test", MessageType: "user_message"})
	got, _ := s.GetMessage(id)
	if got == nil || got.Content != "test" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestGetLastMessage(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	_, _ = s.AddMessage(Message{ThreadID: "t1", FromName: "user", Content: "first", MessageType: "user_message"})
	_, _ = s.AddMessage(Message{ThreadID: "t1", FromName: "cto", Content: "second", MessageType: "agent_message"})
	got, _ := s.GetLastMessage("t1")
	if got == nil || got.Content != "second" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestAddMessage_ParentMessageID(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	parentID, _ := s.AddMessage(Message{ThreadID: "t1", FromName: "user", Content: "request", MessageType: "user_message"})
	_, _ = s.AddMessage(Message{ThreadID: "t1", FromName: "cto", Content: "response", MessageType: "agent_message", ParentMessageID: &parentID})
	msgs, _ := s.ListMessages("t1")
	if msgs[1].ParentMessageID == nil || *msgs[1].ParentMessageID != parentID {
		t.Errorf("parent not set: %v", msgs[1].ParentMessageID)
	}
}

func TestAddSessionEvent_And_List(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	_ = s.AddSessionEvent(SessionEvent{CopilotSessionID: "sess-1", ThreadID: "t1", AgentName: "cto", EventType: "tool_call", Content: "run_terminal"})
	_ = s.AddSessionEvent(SessionEvent{CopilotSessionID: "sess-1", ThreadID: "t1", AgentName: "cto", EventType: "reasoning", Content: "thinking..."})

	events, _ := s.ListSessionEvents("sess-1")
	if len(events) != 2 || events[0].Content != "run_terminal" {
		t.Errorf("unexpected events: %+v", events)
	}
}

func TestListSessionEventsByMessage(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	msgID, _ := s.AddMessage(Message{ThreadID: "t1", FromName: "cto", Content: "done", MessageType: "agent_message", CopilotSessionID: "sess-1"})
	_ = s.AddSessionEvent(SessionEvent{CopilotSessionID: "sess-1", ThreadID: "t1", AgentName: "cto", EventType: "tool_call", Content: "git commit"})

	events, _ := s.ListSessionEventsByMessage(msgID)
	if len(events) != 1 || events[0].Content != "git commit" {
		t.Errorf("unexpected: %+v", events)
	}
}

func TestListSessionEventsByMessage_NoSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	msgID, _ := s.AddMessage(Message{ThreadID: "t1", FromName: "user", Content: "hi", MessageType: "user_message"})
	events, _ := s.ListSessionEventsByMessage(msgID)
	if events != nil {
		t.Errorf("expected nil, got %+v", events)
	}
}

func TestSaveThreadSession_And_Get(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveThreadSession("t1", "cto", "sess-1")
	sid, _ := s.GetThreadSessionID("t1", "cto")
	if sid != "sess-1" {
		t.Errorf("expected sess-1, got %s", sid)
	}
}

func TestSaveThreadSession_Upsert(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveThreadSession("t1", "cto", "old")
	_ = s.SaveThreadSession("t1", "cto", "new")
	sid, _ := s.GetThreadSessionID("t1", "cto")
	if sid != "new" {
		t.Errorf("expected new, got %s", sid)
	}
}

func TestDeleteThreadSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveThreadSession("t1", "cto", "sess")
	_ = s.DeleteThreadSession("t1", "cto")
	sid, _ := s.GetThreadSessionID("t1", "cto")
	if sid != "" {
		t.Errorf("expected empty, got %s", sid)
	}
}

func TestSaveMemory_And_GetMemories(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.SaveMemory(Memory{AgentName: "atlas", Category: "context", Content: "hello world", Source: "test"})
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}
	mems, _ := s.GetMemories(MemoryFilter{AgentName: "atlas"})
	if len(mems) != 1 || mems[0].Content != "hello world" {
		t.Errorf("unexpected: %+v", mems)
	}
}

func TestSaveMemory_MaxPerAgent(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < MaxMemoriesPerAgent; i++ {
		_, _ = s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "m"})
	}
	_, err := s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "overflow"})
	if err == nil || !strings.Contains(err.Error(), "memory limit") {
		t.Errorf("expected memory limit error, got: %v", err)
	}
}

func TestGetMemoriesForPrompt(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "important fact", Source: "test"})
	prompt, _ := s.GetMemoriesForPrompt("a")
	if !strings.Contains(prompt, "important fact") {
		t.Errorf("unexpected prompt: %q", prompt)
	}
}

func createTestProjectAndTask(t *testing.T, s *Store) {
	t.Helper()
	_ = s.CreateProject(Project{ID: "proj", Name: "Proj", Status: "active", CreatedBy: "cto"})
	assigned := "atlas"
	_ = s.CreateTask(Task{ID: "task-1", ProjectID: "proj", Title: "Task 1", Status: "backlog", AssignedTo: &assigned, CreatedBy: "cto", Priority: 2})
}

func TestCreateTask_And_GetTask(t *testing.T) {
	s := newTestStore(t)
	createTestProjectAndTask(t, s)
	got, _ := s.GetTask("task-1")
	if got == nil || got.Title != "Task 1" || got.Priority != 2 {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestAddTaskDependency_CircularDetection(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	_ = s.CreateTask(Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})
	_ = s.CreateTask(Task{ID: "t2", ProjectID: "p", Title: "T2", Status: "backlog", CreatedBy: "x", Priority: 3})
	_ = s.CreateTask(Task{ID: "t3", ProjectID: "p", Title: "T3", Status: "backlog", CreatedBy: "x", Priority: 3})
	_ = s.AddTaskDependency("t1", "t2")
	_ = s.AddTaskDependency("t2", "t3")
	err := s.AddTaskDependency("t3", "t1")
	if err == nil || !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected circular error, got: %v", err)
	}
}

func TestGetContextBriefing(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p", Name: "Test Project", Status: "active", CreatedBy: "cto"})
	assigned := "atlas"
	_ = s.CreateTask(Task{ID: "t1", ProjectID: "p", Title: "Build tests", Status: "in_progress", AssignedTo: &assigned, CreatedBy: "cto", Priority: 1})
	briefing, _ := s.GetContextBriefing("atlas")
	if !strings.Contains(briefing, "Build tests") {
		t.Errorf("unexpected: %q", briefing)
	}
}

func TestForeignKey_TaskWithNonexistentProject(t *testing.T) {
	s := newTestStore(t)
	err := s.CreateTask(Task{ID: "t1", ProjectID: "nonexistent", Title: "T", Status: "backlog", CreatedBy: "x", Priority: 3})
	if err == nil {
		t.Fatal("expected foreign key error")
	}
}

func TestForeignKey_MessageNonexistentThread(t *testing.T) {
	s := newTestStore(t)
	_, err := s.AddMessage(Message{ThreadID: "nonexistent", FromName: "user", Content: "hi", MessageType: "user_message"})
	if err == nil {
		t.Fatal("expected foreign key error")
	}
}

func TestGetActiveThread(t *testing.T) {
	s := newTestStore(t)

	// No threads yet — should return nil.
	got, err := s.GetActiveThread(100)
	if err != nil {
		t.Fatalf("GetActiveThread: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}

	// Create an archived thread — should still return nil.
	_ = s.CreateThread(Thread{ID: "t1", ChatID: 100, Title: "old", Status: "archived"})
	got, err = s.GetActiveThread(100)
	if err != nil {
		t.Fatalf("GetActiveThread: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for archived-only chat, got %+v", got)
	}

	// Create an active thread — should return it.
	_ = s.CreateThread(Thread{ID: "t2", ChatID: 100, Title: "current", Status: "active"})
	got, err = s.GetActiveThread(100)
	if err != nil {
		t.Fatalf("GetActiveThread: %v", err)
	}
	if got == nil || got.ID != "t2" {
		t.Fatalf("expected thread t2, got %+v", got)
	}

	// Different chat should not see the thread.
	got, err = s.GetActiveThread(999)
	if err != nil {
		t.Fatalf("GetActiveThread: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for different chat, got %+v", got)
	}
}

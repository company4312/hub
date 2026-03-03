package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestStore creates a fresh Store backed by a temp SQLite file.
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

// --- Migration ---

func TestNew_CreatesTables(t *testing.T) {
	s := newTestStore(t)
	// Verify key tables exist by running a simple query on each.
	tables := []string{"sessions", "agents", "chat_agents", "activity_log", "memories", "projects", "tasks", "task_dependencies", "task_comments", "schema_migrations"}
	for _, tbl := range tables {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n); err != nil {
			t.Errorf("table %s should exist: %v", tbl, err)
		}
	}
}

func TestNew_Idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	_ = s1.Close()

	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	_ = s2.Close()
}

// --- Agent CRUD ---

func TestRegisterAgent_And_GetAgent(t *testing.T) {
	s := newTestStore(t)
	cfg := AgentConfig{Name: "atlas", Title: "Atlas", SystemPrompt: "You are Atlas.", Model: "gpt-4"}
	if err := s.RegisterAgent(cfg); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	got, err := s.GetAgent("atlas")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got == nil {
		t.Fatal("GetAgent returned nil")
	}
	if got.Name != "atlas" || got.Title != "Atlas" || got.SystemPrompt != "You are Atlas." || got.Model != "gpt-4" {
		t.Errorf("unexpected agent: %+v", got)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetAgent("nonexistent")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestRegisterAgent_Update(t *testing.T) {
	s := newTestStore(t)
	if err := s.RegisterAgent(AgentConfig{Name: "bot", Title: "Bot", SystemPrompt: "v1", Model: "m1"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := s.RegisterAgent(AgentConfig{Name: "bot", Title: "Bot v2", SystemPrompt: "v2", Model: "m2"}); err != nil {
		t.Fatalf("second register: %v", err)
	}
	got, err := s.GetAgent("bot")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.SystemPrompt != "v2" || got.Title != "Bot v2" || got.Model != "m2" {
		t.Errorf("agent not updated: %+v", got)
	}
}

func TestListAgents(t *testing.T) {
	s := newTestStore(t)
	for _, name := range []string{"charlie", "alice", "bob"} {
		if err := s.RegisterAgent(AgentConfig{Name: name, Title: name, SystemPrompt: "p", Model: "m"}); err != nil {
			t.Fatalf("RegisterAgent %s: %v", name, err)
		}
	}
	agents, err := s.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}
	// ListAgents orders by name ASC.
	if agents[0].Name != "alice" || agents[1].Name != "bob" || agents[2].Name != "charlie" {
		t.Errorf("unexpected order: %v, %v, %v", agents[0].Name, agents[1].Name, agents[2].Name)
	}
}

// --- Session management ---

func TestSaveSession_And_GetSessionID(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveSession(1, "cto", "sess-1"); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	sid, err := s.GetSessionID(1, "cto")
	if err != nil {
		t.Fatalf("GetSessionID: %v", err)
	}
	if sid != "sess-1" {
		t.Errorf("expected sess-1, got %s", sid)
	}
}

func TestGetSessionID_NotFound(t *testing.T) {
	s := newTestStore(t)
	sid, err := s.GetSessionID(999, "cto")
	if err != nil {
		t.Fatalf("GetSessionID: %v", err)
	}
	if sid != "" {
		t.Errorf("expected empty, got %s", sid)
	}
}

func TestSaveSession_Upsert(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveSession(1, "cto", "old"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSession(1, "cto", "new"); err != nil {
		t.Fatal(err)
	}
	sid, _ := s.GetSessionID(1, "cto")
	if sid != "new" {
		t.Errorf("expected new, got %s", sid)
	}
}

func TestDeleteSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveSession(1, "cto", "sess")
	if err := s.DeleteSession(1, "cto"); err != nil {
		t.Fatal(err)
	}
	sid, _ := s.GetSessionID(1, "cto")
	if sid != "" {
		t.Errorf("expected empty after delete, got %s", sid)
	}
}

func TestGetActiveAgent_Default(t *testing.T) {
	s := newTestStore(t)
	name, err := s.GetActiveAgent(42)
	if err != nil {
		t.Fatalf("GetActiveAgent: %v", err)
	}
	if name != "cto" {
		t.Errorf("expected default cto, got %s", name)
	}
}

func TestSetActiveAgent_And_Get(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetActiveAgent(42, "atlas"); err != nil {
		t.Fatal(err)
	}
	name, err := s.GetActiveAgent(42)
	if err != nil {
		t.Fatal(err)
	}
	if name != "atlas" {
		t.Errorf("expected atlas, got %s", name)
	}
	// Change it.
	if err := s.SetActiveAgent(42, "bob"); err != nil {
		t.Fatal(err)
	}
	name, _ = s.GetActiveAgent(42)
	if name != "bob" {
		t.Errorf("expected bob, got %s", name)
	}
}

// --- Memory CRUD ---

func TestSaveMemory_And_GetMemories(t *testing.T) {
	s := newTestStore(t)
	id, err := s.SaveMemory(Memory{AgentName: "atlas", Category: "context", Content: "hello world", Source: "test"})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}

	mems, err := s.GetMemories(MemoryFilter{AgentName: "atlas"})
	if err != nil {
		t.Fatalf("GetMemories: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(mems))
	}
	if mems[0].Content != "hello world" || mems[0].Category != "context" {
		t.Errorf("unexpected memory: %+v", mems[0])
	}
}

func TestGetMemories_FilterByCategory(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "c1"})
	_, _ = s.SaveMemory(Memory{AgentName: "a", Category: "decision", Content: "c2"})

	mems, _ := s.GetMemories(MemoryFilter{AgentName: "a", Category: "decision"})
	if len(mems) != 1 || mems[0].Content != "c2" {
		t.Errorf("filter by category failed: %+v", mems)
	}
}

func TestGetMemories_FilterBySearch(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "the quick brown fox"})
	_, _ = s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "lazy dog"})

	mems, _ := s.GetMemories(MemoryFilter{AgentName: "a", Search: "brown"})
	if len(mems) != 1 || !strings.Contains(mems[0].Content, "brown") {
		t.Errorf("search filter failed: %+v", mems)
	}
}

func TestSaveMemory_MaxPerAgent(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < MaxMemoriesPerAgent; i++ {
		if _, err := s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "m"}); err != nil {
			t.Fatalf("SaveMemory %d: %v", i, err)
		}
	}
	_, err := s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "overflow"})
	if err == nil {
		t.Fatal("expected error when exceeding MaxMemoriesPerAgent")
	}
	if !strings.Contains(err.Error(), "memory limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateMemory(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "old"})

	if err := s.UpdateMemory(id, "a", "new"); err != nil {
		t.Fatalf("UpdateMemory: %v", err)
	}
	mems, _ := s.GetMemories(MemoryFilter{AgentName: "a"})
	if len(mems) != 1 || mems[0].Content != "new" {
		t.Errorf("content not updated: %+v", mems)
	}
}

func TestUpdateMemory_WrongAgent(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "x"})

	err := s.UpdateMemory(id, "wrong-agent", "new")
	if err == nil {
		t.Fatal("expected error when updating memory owned by different agent")
	}
}

func TestDeleteMemory(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "x"})

	if err := s.DeleteMemory(id, "a"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	mems, _ := s.GetMemories(MemoryFilter{AgentName: "a"})
	if len(mems) != 0 {
		t.Errorf("expected 0 memories after delete, got %d", len(mems))
	}
}

func TestDeleteMemory_WrongAgent(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "x"})

	err := s.DeleteMemory(id, "wrong-agent")
	if err == nil {
		t.Fatal("expected error when deleting memory owned by different agent")
	}
}

func TestGetMemoriesForPrompt(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.SaveMemory(Memory{AgentName: "a", Category: "context", Content: "important fact", Source: "test"})
	_, _ = s.SaveMemory(Memory{AgentName: "a", Category: "decision", Content: "we chose X"})

	prompt, err := s.GetMemoriesForPrompt("a")
	if err != nil {
		t.Fatalf("GetMemoriesForPrompt: %v", err)
	}
	if !strings.Contains(prompt, "[Your Memories]") {
		t.Error("missing header")
	}
	if !strings.Contains(prompt, "important fact") {
		t.Error("missing memory content")
	}
	if !strings.Contains(prompt, "(source: test)") {
		t.Error("missing source")
	}
}

func TestGetMemoriesForPrompt_Empty(t *testing.T) {
	s := newTestStore(t)
	prompt, err := s.GetMemoriesForPrompt("nobody")
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "" {
		t.Errorf("expected empty prompt, got %q", prompt)
	}
}

func TestValidMemoryCategories(t *testing.T) {
	expected := []string{"lesson_learned", "preference", "context", "decision", "skill", "other"}
	for _, cat := range expected {
		if !ValidMemoryCategories[cat] {
			t.Errorf("category %q should be valid", cat)
		}
	}
	if ValidMemoryCategories["bogus"] {
		t.Error("bogus should not be valid")
	}
}

// --- Activity log ---

func TestLogActivity_And_GetActivities(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Format(time.RFC3339)
	entry := ActivityEntry{
		Timestamp: now,
		AgentName: "atlas",
		EventType: "memory_created",
		Content:   "test content",
		Metadata:  `{"key":"val"}`,
		ChatID:    1,
	}
	if err := s.LogActivity(entry); err != nil {
		t.Fatalf("LogActivity: %v", err)
	}

	entries, err := s.GetActivities(ActivityFilter{})
	if err != nil {
		t.Fatalf("GetActivities: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].AgentName != "atlas" || entries[0].EventType != "memory_created" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}

func TestGetActivities_Filters(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.LogActivity(ActivityEntry{Timestamp: now, AgentName: "a", EventType: "chat", Content: "c1"})
	_ = s.LogActivity(ActivityEntry{Timestamp: now, AgentName: "b", EventType: "memory_created", Content: "c2"})
	_ = s.LogActivity(ActivityEntry{Timestamp: now, AgentName: "a", EventType: "memory_created", Content: "c3"})

	tests := []struct {
		name   string
		filter ActivityFilter
		want   int
	}{
		{"by agent", ActivityFilter{AgentName: "a"}, 2},
		{"by type", ActivityFilter{EventType: "chat"}, 1},
		{"by agent+type", ActivityFilter{AgentName: "a", EventType: "memory_created"}, 1},
		{"limit", ActivityFilter{Limit: 1}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := s.GetActivities(tc.filter)
			if err != nil {
				t.Fatalf("GetActivities: %v", err)
			}
			if len(entries) != tc.want {
				t.Errorf("expected %d entries, got %d", tc.want, len(entries))
			}
		})
	}
}

// --- Project CRUD ---

func TestCreateProject_And_GetProject(t *testing.T) {
	s := newTestStore(t)
	p := Project{ID: "proj-1", Name: "My Project", Description: "desc", Status: "active", CreatedBy: "cto"}
	if err := s.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	got, err := s.GetProject("proj-1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got == nil {
		t.Fatal("GetProject returned nil")
	}
	if got.Name != "My Project" || got.Status != "active" {
		t.Errorf("unexpected project: %+v", got)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetProject("nope")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestListProjects(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p1", Name: "P1", Status: "active", CreatedBy: "x"})
	_ = s.CreateProject(Project{ID: "p2", Name: "P2", Status: "active", CreatedBy: "x"})

	projects, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2, got %d", len(projects))
	}
	// Verify both projects are present.
	ids := map[string]bool{projects[0].ID: true, projects[1].ID: true}
	if !ids["p1"] || !ids["p2"] {
		t.Errorf("expected p1 and p2, got %v", ids)
	}
}

func TestUpdateProjectStatus(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p1", Name: "P", Status: "active", CreatedBy: "x"})

	if err := s.UpdateProjectStatus("p1", "completed"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetProject("p1")
	if got.Status != "completed" {
		t.Errorf("expected completed, got %s", got.Status)
	}
}

func TestUpdateProjectStatus_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateProjectStatus("nope", "active")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
}

func TestValidProjectStatuses(t *testing.T) {
	for _, st := range []string{"active", "completed", "archived"} {
		if !ValidProjectStatuses[st] {
			t.Errorf("%q should be valid", st)
		}
	}
	if ValidProjectStatuses["invalid"] {
		t.Error("invalid should not be valid")
	}
}

// --- Task CRUD ---

func createTestProjectAndTask(t *testing.T, s *Store) {
	t.Helper()
	_ = s.CreateProject(Project{ID: "proj", Name: "Proj", Status: "active", CreatedBy: "cto"})
	assigned := "atlas"
	_ = s.CreateTask(Task{ID: "task-1", ProjectID: "proj", Title: "Task 1", Status: "backlog", AssignedTo: &assigned, CreatedBy: "cto", Priority: 2})
}

func TestCreateTask_And_GetTask(t *testing.T) {
	s := newTestStore(t)
	createTestProjectAndTask(t, s)

	got, err := s.GetTask("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetTask returned nil")
	}
	if got.Title != "Task 1" || got.ProjectID != "proj" || got.Priority != 2 {
		t.Errorf("unexpected task: %+v", got)
	}
	if got.AssignedTo == nil || *got.AssignedTo != "atlas" {
		t.Errorf("assigned_to mismatch: %v", got.AssignedTo)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetTask("nope")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestListTasks_Filters(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p1", Name: "P1", Status: "active", CreatedBy: "x"})
	_ = s.CreateProject(Project{ID: "p2", Name: "P2", Status: "active", CreatedBy: "x"})
	a1 := "alice"
	a2 := "bob"
	_ = s.CreateTask(Task{ID: "t1", ProjectID: "p1", Title: "T1", Status: "backlog", AssignedTo: &a1, CreatedBy: "x", Priority: 1})
	_ = s.CreateTask(Task{ID: "t2", ProjectID: "p1", Title: "T2", Status: "in_progress", AssignedTo: &a2, CreatedBy: "x", Priority: 2})
	_ = s.CreateTask(Task{ID: "t3", ProjectID: "p2", Title: "T3", Status: "backlog", AssignedTo: &a1, CreatedBy: "x", Priority: 3})

	tests := []struct {
		name   string
		filter TaskFilter
		want   int
	}{
		{"by project", TaskFilter{ProjectID: "p1"}, 2},
		{"by status", TaskFilter{Status: "backlog"}, 2},
		{"by assigned", TaskFilter{AssignedTo: "alice"}, 2},
		{"by limit", TaskFilter{Limit: 1}, 1},
		{"all", TaskFilter{}, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tasks, err := s.ListTasks(tc.filter)
			if err != nil {
				t.Fatalf("ListTasks: %v", err)
			}
			if len(tasks) != tc.want {
				t.Errorf("expected %d, got %d", tc.want, len(tasks))
			}
		})
	}
}

func TestUpdateTask(t *testing.T) {
	s := newTestStore(t)
	createTestProjectAndTask(t, s)

	task, _ := s.GetTask("task-1")
	task.Title = "Updated Title"
	task.Status = "in_progress"
	if err := s.UpdateTask(*task); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTask("task-1")
	if got.Title != "Updated Title" || got.Status != "in_progress" {
		t.Errorf("task not updated: %+v", got)
	}
}

func TestUpdateTask_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateTask(Task{ID: "nope", ProjectID: "p", Title: "t", Status: "backlog", CreatedBy: "x", Priority: 3})
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	s := newTestStore(t)
	createTestProjectAndTask(t, s)

	if err := s.UpdateTaskStatus("task-1", "done"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTask("task-1")
	if got.Status != "done" {
		t.Errorf("expected done, got %s", got.Status)
	}
}

func TestUpdateTaskStatus_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateTaskStatus("nope", "done")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidTaskStatuses(t *testing.T) {
	for _, st := range []string{"backlog", "todo", "in_progress", "review", "done"} {
		if !ValidTaskStatuses[st] {
			t.Errorf("%q should be valid", st)
		}
	}
	if ValidTaskStatuses["invalid"] {
		t.Error("invalid should not be valid")
	}
}

// --- Task dependencies ---

func TestAddTaskDependency_And_Get(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	_ = s.CreateTask(Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})
	_ = s.CreateTask(Task{ID: "t2", ProjectID: "p", Title: "T2", Status: "backlog", CreatedBy: "x", Priority: 3})

	if err := s.AddTaskDependency("t1", "t2"); err != nil {
		t.Fatalf("AddTaskDependency: %v", err)
	}
	deps, err := s.GetTaskDependencies("t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0] != "t2" {
		t.Errorf("unexpected deps: %v", deps)
	}
}

func TestGetBlockingTasks(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	_ = s.CreateTask(Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})
	_ = s.CreateTask(Task{ID: "t2", ProjectID: "p", Title: "T2", Status: "backlog", CreatedBy: "x", Priority: 3})
	_ = s.CreateTask(Task{ID: "t3", ProjectID: "p", Title: "T3", Status: "done", CreatedBy: "x", Priority: 3})
	_ = s.AddTaskDependency("t1", "t2")
	_ = s.AddTaskDependency("t1", "t3")

	blocking, err := s.GetBlockingTasks("t1")
	if err != nil {
		t.Fatal(err)
	}
	// Only t2 blocks (t3 is done).
	if len(blocking) != 1 || blocking[0].ID != "t2" {
		t.Errorf("expected [t2], got %+v", blocking)
	}
}

func TestAddTaskDependency_CircularDetection(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	_ = s.CreateTask(Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})
	_ = s.CreateTask(Task{ID: "t2", ProjectID: "p", Title: "T2", Status: "backlog", CreatedBy: "x", Priority: 3})
	_ = s.CreateTask(Task{ID: "t3", ProjectID: "p", Title: "T3", Status: "backlog", CreatedBy: "x", Priority: 3})

	// t1 -> t2 -> t3
	_ = s.AddTaskDependency("t1", "t2")
	_ = s.AddTaskDependency("t2", "t3")

	// t3 -> t1 would create a cycle.
	err := s.AddTaskDependency("t3", "t1")
	if err == nil {
		t.Fatal("expected circular dependency error")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Task comments ---

func TestAddTaskComment_And_Get(t *testing.T) {
	s := newTestStore(t)
	createTestProjectAndTask(t, s)

	id, err := s.AddTaskComment(TaskComment{TaskID: "task-1", AgentName: "atlas", Content: "working on it"})
	if err != nil {
		t.Fatalf("AddTaskComment: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}

	comments, err := s.GetTaskComments("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	if comments[0].Content != "working on it" || comments[0].AgentName != "atlas" {
		t.Errorf("unexpected comment: %+v", comments[0])
	}
}

func TestGetTaskComments_Empty(t *testing.T) {
	s := newTestStore(t)
	createTestProjectAndTask(t, s)

	comments, err := s.GetTaskComments("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if comments != nil {
		t.Errorf("expected nil, got %+v", comments)
	}
}

// --- Context briefing ---

func TestGetContextBriefing(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p", Name: "Test Project", Status: "active", CreatedBy: "cto"})
	assigned := "atlas"
	_ = s.CreateTask(Task{ID: "t1", ProjectID: "p", Title: "Build tests", Status: "in_progress", AssignedTo: &assigned, CreatedBy: "cto", Priority: 1})
	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.LogActivity(ActivityEntry{Timestamp: now, AgentName: "atlas", EventType: "chat", Content: "hi"})

	briefing, err := s.GetContextBriefing("atlas")
	if err != nil {
		t.Fatalf("GetContextBriefing: %v", err)
	}
	if !strings.Contains(briefing, "[Current Context]") {
		t.Error("missing header")
	}
	if !strings.Contains(briefing, "Build tests") {
		t.Error("missing assigned task")
	}
	if !strings.Contains(briefing, "Test Project") {
		t.Error("missing project")
	}
	if !strings.Contains(briefing, "atlas") {
		t.Error("missing activity")
	}
}

func TestGetContextBriefing_Empty(t *testing.T) {
	s := newTestStore(t)
	briefing, err := s.GetContextBriefing("nobody")
	if err != nil {
		t.Fatal(err)
	}
	if briefing != "" {
		t.Errorf("expected empty, got %q", briefing)
	}
}

// --- Foreign key enforcement ---

func TestForeignKey_TaskWithNonexistentProject(t *testing.T) {
	s := newTestStore(t)
	err := s.CreateTask(Task{ID: "t1", ProjectID: "nonexistent", Title: "T", Status: "backlog", CreatedBy: "x", Priority: 3})
	if err == nil {
		t.Fatal("expected foreign key error for nonexistent project_id")
	}
}

func TestForeignKey_TaskDependencyNonexistentTask(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateProject(Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	_ = s.CreateTask(Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})

	err := s.AddTaskDependency("t1", "nonexistent")
	if err == nil {
		t.Fatal("expected foreign key error for nonexistent depends_on task")
	}
}

func TestForeignKey_TaskCommentNonexistentTask(t *testing.T) {
	s := newTestStore(t)
	_, err := s.AddTaskComment(TaskComment{TaskID: "nonexistent", AgentName: "a", Content: "c"})
	if err == nil {
		t.Fatal("expected foreign key error for nonexistent task_id in comment")
	}
}

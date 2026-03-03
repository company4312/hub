package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/company4312/copilot-telegram-bot/internal/store"
)

// newTestServer creates an API server backed by a temp store, returning the
// underlying *http.ServeMux handler (bypassing net.Listen).
func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	srv := New(s, ":0")
	return srv, s
}

// do is a test helper that sends a request through the server's handler.
func do(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rr, req)
	return rr
}

// --- GET /api/agents ---

func TestGetAgents_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/api/agents", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rr.Code)
	}
	var agents []map[string]string
	json.Unmarshal(rr.Body.Bytes(), &agents)
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestGetAgents_WithRegistered(t *testing.T) {
	srv, s := newTestServer(t)
	s.RegisterAgent(store.AgentConfig{Name: "atlas", Title: "Atlas", SystemPrompt: "p", Model: "m"})

	rr := do(t, srv, http.MethodGet, "/api/agents", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var agents []map[string]string
	json.Unmarshal(rr.Body.Bytes(), &agents)
	if len(agents) != 1 || agents[0]["name"] != "atlas" || agents[0]["title"] != "Atlas" {
		t.Errorf("unexpected agents: %v", agents)
	}
}

// --- POST /api/memories ---

func TestCreateMemory(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]string{
		"agent_name": "atlas",
		"category":   "context",
		"content":    "test memory",
		"source":     "unit-test",
	}
	rr := do(t, srv, http.MethodPost, "/api/memories", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]float64
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["id"] <= 0 {
		t.Errorf("expected positive id, got %v", resp["id"])
	}
}

func TestCreateMemory_MissingFields(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/api/memories", map[string]string{"agent_name": "a"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCreateMemory_InvalidCategory(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]string{
		"agent_name": "a",
		"category":   "bogus",
		"content":    "c",
	}
	rr := do(t, srv, http.MethodPost, "/api/memories", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- POST /api/projects ---

func TestCreateProject(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]string{
		"id":          "proj-1",
		"name":        "My Project",
		"description": "desc",
		"created_by":  "cto",
	}
	rr := do(t, srv, http.MethodPost, "/api/projects", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["id"] != "proj-1" {
		t.Errorf("unexpected id: %v", resp["id"])
	}
}

func TestCreateProject_MissingFields(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/api/projects", map[string]string{"id": "p"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCreateProject_NameTooLong(t *testing.T) {
	srv, _ := newTestServer(t)
	long := make([]byte, store.MaxTitleLength+1)
	for i := range long {
		long[i] = 'a'
	}
	body := map[string]string{
		"id":         "p",
		"name":       string(long),
		"created_by": "cto",
	}
	rr := do(t, srv, http.MethodPost, "/api/projects", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- GET /api/projects ---

func TestListProjects(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p1", Name: "P1", Status: "active", CreatedBy: "x"})
	s.CreateProject(store.Project{ID: "p2", Name: "P2", Status: "active", CreatedBy: "x"})

	rr := do(t, srv, http.MethodGet, "/api/projects", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var projects []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &projects)
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

// --- POST /api/projects/{id}/tasks ---

func TestCreateTaskInProject(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "proj", Name: "P", Status: "active", CreatedBy: "x"})

	body := map[string]any{
		"id":         "task-1",
		"title":      "My Task",
		"created_by": "atlas",
		"priority":   2,
	}
	rr := do(t, srv, http.MethodPost, "/api/projects/proj/tasks", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["id"] != "task-1" {
		t.Errorf("unexpected id: %v", resp["id"])
	}
}

func TestCreateTask_ProjectNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]any{
		"id":         "task-1",
		"title":      "T",
		"created_by": "x",
	}
	rr := do(t, srv, http.MethodPost, "/api/projects/nonexistent/tasks", body)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestCreateTask_TitleTooLong(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "proj", Name: "P", Status: "active", CreatedBy: "x"})

	long := make([]byte, store.MaxTitleLength+1)
	for i := range long {
		long[i] = 'a'
	}
	body := map[string]any{
		"id":         "task-1",
		"title":      string(long),
		"created_by": "x",
	}
	rr := do(t, srv, http.MethodPost, "/api/projects/proj/tasks", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- GET /api/tasks?status=backlog ---

func TestListTasks_FilterByStatus(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})
	s.CreateTask(store.Task{ID: "t2", ProjectID: "p", Title: "T2", Status: "in_progress", CreatedBy: "x", Priority: 3})

	rr := do(t, srv, http.MethodGet, "/api/tasks?status=backlog", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var tasks []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &tasks)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0]["id"] != "t1" {
		t.Errorf("expected t1, got %v", tasks[0]["id"])
	}
}

// --- PATCH /api/tasks/{id} ---

func TestPatchTask(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "Old", Status: "backlog", CreatedBy: "x", Priority: 3})

	newTitle := "New Title"
	body := map[string]any{"title": newTitle}
	rr := do(t, srv, http.MethodPatch, "/api/tasks/t1", body)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}

	// Verify.
	task, _ := s.GetTask("t1")
	if task.Title != "New Title" {
		t.Errorf("expected 'New Title', got %q", task.Title)
	}
}

func TestPatchTask_InvalidStatus(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "T", Status: "backlog", CreatedBy: "x", Priority: 3})

	body := map[string]any{"status": "invalid_status"}
	rr := do(t, srv, http.MethodPatch, "/api/tasks/t1", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestPatchTask_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPatch, "/api/tasks/nope", map[string]any{"title": "x"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestPatchTask_TitleTooLong(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "T", Status: "backlog", CreatedBy: "x", Priority: 3})

	long := make([]byte, store.MaxTitleLength+1)
	for i := range long {
		long[i] = 'a'
	}
	body := map[string]any{"title": string(long)}
	rr := do(t, srv, http.MethodPatch, "/api/tasks/t1", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- POST /api/tasks/{id}/comments ---

func TestAddTaskComment(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "T", Status: "backlog", CreatedBy: "x", Priority: 3})

	body := map[string]string{"agent_name": "atlas", "content": "looks good"}
	rr := do(t, srv, http.MethodPost, "/api/tasks/t1/comments", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]float64
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["id"] <= 0 {
		t.Errorf("expected positive id, got %v", resp["id"])
	}
}

func TestAddTaskComment_TaskNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]string{"agent_name": "a", "content": "c"}
	rr := do(t, srv, http.MethodPost, "/api/tasks/nope/comments", body)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// --- POST /api/tasks/{id}/dependencies ---

func TestAddTaskDependency(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})
	s.CreateTask(store.Task{ID: "t2", ProjectID: "p", Title: "T2", Status: "backlog", CreatedBy: "x", Priority: 3})

	body := map[string]string{"depends_on": "t2"}
	rr := do(t, srv, http.MethodPost, "/api/tasks/t1/dependencies", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestAddTaskDependency_SelfDependency(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})

	body := map[string]string{"depends_on": "t1"}
	rr := do(t, srv, http.MethodPost, "/api/tasks/t1/dependencies", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for self-dependency, got %d", rr.Code)
	}
}

func TestAddTaskDependency_DepNotFound(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "T1", Status: "backlog", CreatedBy: "x", Priority: 3})

	body := map[string]string{"depends_on": "nonexistent"}
	rr := do(t, srv, http.MethodPost, "/api/tasks/t1/dependencies", body)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// --- Validation ---

func TestCreateMemory_ContentTooLong(t *testing.T) {
	srv, _ := newTestServer(t)
	long := make([]byte, store.MaxMemoryContent+1)
	for i := range long {
		long[i] = 'a'
	}
	body := map[string]string{
		"agent_name": "a",
		"category":   "context",
		"content":    string(long),
	}
	rr := do(t, srv, http.MethodPost, "/api/memories", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestPatchProject_InvalidStatus(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p1", Name: "P", Status: "active", CreatedBy: "x"})

	body := map[string]string{"status": "bogus"}
	rr := do(t, srv, http.MethodPatch, "/api/projects/p1", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommentTooLong(t *testing.T) {
	srv, s := newTestServer(t)
	s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "T", Status: "backlog", CreatedBy: "x", Priority: 3})

	long := make([]byte, store.MaxCommentLength+1)
	for i := range long {
		long[i] = 'a'
	}
	body := map[string]string{"agent_name": "a", "content": string(long)}
	rr := do(t, srv, http.MethodPost, "/api/tasks/t1/comments", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAgents_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/api/agents", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

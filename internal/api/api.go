package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/company4312/copilot-telegram-bot/internal/store"
	"github.com/company4312/copilot-telegram-bot/web"
)

// AgentPool is the interface the API server uses to interact with agents.
// It is satisfied by *agent.Pool but kept as an interface to avoid a
// circular import and to make testing simpler.
type AgentPool interface {
	SendMessageTo(ctx context.Context, agentName string, chatID int64, text string) (string, error)
	DelegateTask(ctx context.Context, fromAgent, toAgent string, chatID int64, projectID, taskID, title, instructions string, priority int) error
}

// Server is the HTTP API server for the Company4312 dashboard.
type Server struct {
	store      *store.Store
	pool       AgentPool
	addr       string
	httpServer *http.Server

	mu      sync.Mutex
	clients map[chan store.ActivityEntry]struct{}
}

// New creates a new API server. pool may be nil (endpoints that require it
// will return 503).
func New(s *store.Store, pool AgentPool, addr string) *Server {
	srv := &Server{
		store:   s,
		pool:    pool,
		addr:    addr,
		clients: make(map[chan store.ActivityEntry]struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents/", srv.handleAgentRoutes)
	mux.HandleFunc("/api/agents", srv.handleAgents)
	mux.HandleFunc("/api/activity/stream", srv.handleActivityStream)
	mux.HandleFunc("/api/activity", srv.handleActivity)
	mux.HandleFunc("/api/memories/", srv.handleMemoryByID)
	mux.HandleFunc("/api/memories", srv.handleMemories)
	mux.HandleFunc("/api/projects/", srv.handleProjectByID)
	mux.HandleFunc("/api/projects", srv.handleProjects)
	mux.HandleFunc("/api/tasks/delegate", srv.handleDelegateTask)
	mux.HandleFunc("/api/tasks/", srv.handleTaskByID)
	mux.HandleFunc("/api/tasks", srv.handleTasks)

	// Serve embedded frontend assets.
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Fatalf("embed frontend: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(distFS)))

	srv.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return srv
}

// Start begins listening for HTTP requests.
func (srv *Server) Start() error {
	ln, err := net.Listen("tcp", srv.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.addr, err)
	}
	log.Printf("Dashboard API listening on %s", srv.addr)
	go func() {
		if err := srv.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("dashboard server error: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (srv *Server) Stop(ctx context.Context) error {
	return srv.httpServer.Shutdown(ctx)
}

// Broadcast sends an activity entry to all connected SSE clients.
func (srv *Server) Broadcast(entry store.ActivityEntry) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for ch := range srv.clients {
		select {
		case ch <- entry:
		default:
			// Drop if client is slow.
		}
	}
}

func (srv *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents, err := srv.store.ListAgents()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("list agents: %v", err)
		return
	}

	type agentInfo struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	out := make([]agentInfo, len(agents))
	for i, a := range agents {
		out[i] = agentInfo{Name: a.Name, Title: a.Title}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (srv *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	filter := store.ActivityFilter{
		AgentName: q.Get("agent"),
		EventType: q.Get("type"),
		Since:     q.Get("since"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	entries, err := srv.store.GetActivities(filter)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get activities: %v", err)
		return
	}
	if entries == nil {
		entries = []store.ActivityEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

func (srv *Server) handleActivityStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ch := make(chan store.ActivityEntry, 64)
	srv.mu.Lock()
	srv.clients[ch] = struct{}{}
	srv.mu.Unlock()

	defer func() {
		srv.mu.Lock()
		delete(srv.clients, ch)
		srv.mu.Unlock()
		close(ch)
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-ch:
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (srv *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		srv.handleListMemories(w, r)
	case http.MethodPost:
		srv.handleCreateMemory(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.MemoryFilter{
		AgentName: q.Get("agent"),
		Category:  q.Get("category"),
		Search:    q.Get("search"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	memories, err := srv.store.GetMemories(filter)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get memories: %v", err)
		return
	}
	if memories == nil {
		memories = []store.Memory{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(memories)
}

func (srv *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentName string `json:"agent_name"`
		Category  string `json:"category"`
		Content   string `json:"content"`
		Source    string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.AgentName == "" || body.Category == "" || body.Content == "" {
		http.Error(w, "agent_name, category, and content are required", http.StatusBadRequest)
		return
	}
	if !store.ValidMemoryCategories[body.Category] {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}
	if len(body.Content) > store.MaxMemoryContent {
		http.Error(w, "content too long", http.StatusBadRequest)
		return
	}
	if len(body.Source) > 200 {
		http.Error(w, "source too long", http.StatusBadRequest)
		return
	}
	if len(body.AgentName) > 100 {
		http.Error(w, "agent_name too long", http.StatusBadRequest)
		return
	}

	id, err := srv.store.SaveMemory(store.Memory{
		AgentName: body.AgentName,
		Category:  body.Category,
		Content:   body.Content,
		Source:    body.Source,
	})
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Printf("save memory: %v", err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	metadata, _ := json.Marshal(map[string]any{
		"category":  body.Category,
		"source":    body.Source,
		"memory_id": id,
	})
	entry := store.ActivityEntry{
		Timestamp: now,
		AgentName: body.AgentName,
		EventType: "memory_created",
		Content:   body.Content,
		Metadata:  string(metadata),
	}
	if err := srv.store.LogActivity(entry); err != nil {
		log.Printf("log activity: %v", err)
	}
	srv.Broadcast(entry)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]int64{"id": id})
}

func (srv *Server) handleMemoryByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/memories/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid memory ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		srv.handleUpdateMemory(w, r, id)
	case http.MethodDelete:
		srv.handleDeleteMemory(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleUpdateMemory(w http.ResponseWriter, r *http.Request, id int64) {
	var body struct {
		AgentName string `json:"agent_name"`
		Content   string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Content == "" || body.AgentName == "" {
		http.Error(w, "agent_name and content are required", http.StatusBadRequest)
		return
	}
	if len(body.Content) > store.MaxMemoryContent {
		http.Error(w, "content too long", http.StatusBadRequest)
		return
	}

	if err := srv.store.UpdateMemory(id, body.AgentName, body.Content); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Printf("update memory: %v", err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	metadata, _ := json.Marshal(map[string]any{"memory_id": id})
	entry := store.ActivityEntry{
		Timestamp: now,
		AgentName: body.AgentName,
		EventType: "memory_updated",
		Content:   body.Content,
		Metadata:  string(metadata),
	}
	if err := srv.store.LogActivity(entry); err != nil {
		log.Printf("log activity: %v", err)
	}
	srv.Broadcast(entry)

	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request, id int64) {
	var body struct {
		AgentName string `json:"agent_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AgentName == "" {
		http.Error(w, "agent_name is required", http.StatusBadRequest)
		return
	}

	if err := srv.store.DeleteMemory(id, body.AgentName); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Printf("delete memory: %v", err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	metadata, _ := json.Marshal(map[string]any{"memory_id": id})
	entry := store.ActivityEntry{
		Timestamp: now,
		AgentName: body.AgentName,
		EventType: "memory_deleted",
		Content:   fmt.Sprintf("memory %d deleted", id),
		Metadata:  string(metadata),
	}
	if err := srv.store.LogActivity(entry); err != nil {
		log.Printf("log activity: %v", err)
	}
	srv.Broadcast(entry)

	w.WriteHeader(http.StatusNoContent)
}

// --- Project handlers ---

func (srv *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		srv.handleListProjects(w, r)
	case http.MethodPost:
		srv.handleCreateProject(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleListProjects(w http.ResponseWriter, _ *http.Request) {
	projects, err := srv.store.ListProjects()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("list projects: %v", err)
		return
	}
	if projects == nil {
		projects = []store.Project{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(projects)
}

func (srv *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		CreatedBy   string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.ID == "" || body.Name == "" || body.CreatedBy == "" {
		http.Error(w, "id, name, and created_by are required", http.StatusBadRequest)
		return
	}
	if len(body.Name) > store.MaxTitleLength {
		http.Error(w, "name too long", http.StatusBadRequest)
		return
	}
	if len(body.Description) > store.MaxDescriptionLength {
		http.Error(w, "description too long", http.StatusBadRequest)
		return
	}
	if len(body.ID) > store.MaxTitleLength {
		http.Error(w, "id too long", http.StatusBadRequest)
		return
	}

	if err := srv.store.CreateProject(store.Project{
		ID:          body.ID,
		Name:        body.Name,
		Description: body.Description,
		Status:      "active",
		CreatedBy:   body.CreatedBy,
	}); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Printf("create project: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": body.ID})
}

func (srv *Server) handleProjectByID(w http.ResponseWriter, r *http.Request) {
	// Route: /api/projects/{id} or /api/projects/{id}/tasks
	path := r.URL.Path[len("/api/projects/"):]

	// Check for sub-routes: {id}/tasks
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "project id required", http.StatusBadRequest)
		return
	}

	projectID := parts[0]

	if len(parts) == 2 && parts[1] == "tasks" {
		srv.handleProjectTasks(w, r, projectID)
		return
	}

	if len(parts) > 1 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		srv.handleGetProject(w, r, projectID)
	case http.MethodPatch:
		srv.handlePatchProject(w, r, projectID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleGetProject(w http.ResponseWriter, _ *http.Request, id string) {
	p, err := srv.store.GetProject(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get project: %v", err)
		return
	}
	if p == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (srv *Server) handlePatchProject(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Status == "" {
		http.Error(w, "status is required", http.StatusBadRequest)
		return
	}
	if !store.ValidProjectStatuses[body.Status] {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := srv.store.UpdateProjectStatus(id, body.Status); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Printf("update project status: %v", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handleProjectTasks(w http.ResponseWriter, r *http.Request, projectID string) {
	switch r.Method {
	case http.MethodGet:
		tasks, err := srv.store.ListTasks(store.TaskFilter{ProjectID: projectID})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Printf("list project tasks: %v", err)
			return
		}
		if tasks == nil {
			tasks = []store.Task{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tasks)
	case http.MethodPost:
		srv.handleCreateTaskForProject(w, r, projectID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleCreateTaskForProject(w http.ResponseWriter, r *http.Request, projectID string) {
	var body struct {
		ID          string  `json:"id"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		AssignedTo  *string `json:"assigned_to"`
		CreatedBy   string  `json:"created_by"`
		Priority    int     `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.ID == "" || body.Title == "" || body.CreatedBy == "" {
		http.Error(w, "id, title, and created_by are required", http.StatusBadRequest)
		return
	}
	if len(body.Title) > store.MaxTitleLength {
		http.Error(w, "title too long", http.StatusBadRequest)
		return
	}
	if len(body.Description) > store.MaxDescriptionLength {
		http.Error(w, "description too long", http.StatusBadRequest)
		return
	}
	if body.Priority < 1 || body.Priority > 4 {
		body.Priority = 3
	}

	// Verify project exists.
	p, err := srv.store.GetProject(projectID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get project: %v", err)
		return
	}
	if p == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	if err := srv.store.CreateTask(store.Task{
		ID:          body.ID,
		ProjectID:   projectID,
		Title:       body.Title,
		Description: body.Description,
		Status:      "backlog",
		AssignedTo:  body.AssignedTo,
		CreatedBy:   body.CreatedBy,
		Priority:    body.Priority,
	}); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Printf("create task: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": body.ID})
}

// --- Task handlers ---

func (srv *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	filter := store.TaskFilter{
		ProjectID:  q.Get("project_id"),
		Status:     q.Get("status"),
		AssignedTo: q.Get("assigned_to"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	tasks, err := srv.store.ListTasks(filter)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("list tasks: %v", err)
		return
	}
	if tasks == nil {
		tasks = []store.Task{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks)
}

func (srv *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/tasks/"):]
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}

	taskID := parts[0]

	if len(parts) == 2 {
		switch parts[1] {
		case "status":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			srv.handleTaskStatus(w, r, taskID)
			return
		case "dependencies":
			srv.handleTaskDependencies(w, r, taskID)
			return
		case "comments":
			srv.handleTaskComments(w, r, taskID)
			return
		}
	}

	if len(parts) > 1 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		srv.handleGetTask(w, r, taskID)
	case http.MethodPatch:
		srv.handlePatchTask(w, r, taskID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleGetTask(w http.ResponseWriter, _ *http.Request, id string) {
	t, err := srv.store.GetTask(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get task: %v", err)
		return
	}
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(t)
}

func (srv *Server) handlePatchTask(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := srv.store.GetTask(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get task: %v", err)
		return
	}
	if existing == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	var body struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		AssignedTo  *string `json:"assigned_to"`
		Priority    *int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if body.Title != nil {
		if len(*body.Title) > store.MaxTitleLength {
			http.Error(w, "title too long", http.StatusBadRequest)
			return
		}
		existing.Title = *body.Title
	}
	if body.Description != nil {
		if len(*body.Description) > store.MaxDescriptionLength {
			http.Error(w, "description too long", http.StatusBadRequest)
			return
		}
		existing.Description = *body.Description
	}
	if body.Status != nil {
		if !store.ValidTaskStatuses[*body.Status] {
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}
		existing.Status = *body.Status
	}
	if body.AssignedTo != nil {
		existing.AssignedTo = body.AssignedTo
	}
	if body.Priority != nil {
		if *body.Priority < 1 || *body.Priority > 4 {
			http.Error(w, "priority must be 1-4", http.StatusBadRequest)
			return
		}
		existing.Priority = *body.Priority
	}

	if err := srv.store.UpdateTask(*existing); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Printf("update task: %v", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handleTaskDependencies(w http.ResponseWriter, r *http.Request, taskID string) {
	switch r.Method {
	case http.MethodGet:
		blocking, err := srv.store.GetBlockingTasks(taskID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Printf("get blocking tasks: %v", err)
			return
		}
		if blocking == nil {
			blocking = []store.Task{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(blocking)
	case http.MethodPost:
		var body struct {
			DependsOn string `json:"depends_on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.DependsOn == "" {
			http.Error(w, "depends_on is required", http.StatusBadRequest)
			return
		}
		if body.DependsOn == taskID {
			http.Error(w, "a task cannot depend on itself", http.StatusBadRequest)
			return
		}
		// Verify both tasks exist.
		t, err := srv.store.GetTask(taskID)
		if err != nil || t == nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		dep, err := srv.store.GetTask(body.DependsOn)
		if err != nil || dep == nil {
			http.Error(w, "dependency task not found", http.StatusNotFound)
			return
		}
		if err := srv.store.AddTaskDependency(taskID, body.DependsOn); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			log.Printf("add task dependency: %v", err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleTaskComments(w http.ResponseWriter, r *http.Request, taskID string) {
	switch r.Method {
	case http.MethodGet:
		comments, err := srv.store.GetTaskComments(taskID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Printf("get task comments: %v", err)
			return
		}
		if comments == nil {
			comments = []store.TaskComment{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(comments)
	case http.MethodPost:
		var body struct {
			AgentName string `json:"agent_name"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.AgentName == "" || body.Content == "" {
			http.Error(w, "agent_name and content are required", http.StatusBadRequest)
			return
		}
		if len(body.Content) > store.MaxCommentLength {
			http.Error(w, "comment too long", http.StatusBadRequest)
			return
		}
		// Verify task exists.
		t, err := srv.store.GetTask(taskID)
		if err != nil || t == nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		id, err := srv.store.AddTaskComment(store.TaskComment{
			TaskID:    taskID,
			AgentName: body.AgentName,
			Content:   body.Content,
		})
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			log.Printf("add task comment: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]int64{"id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Agent message / delegate handlers ---

func (srv *Server) handleAgentRoutes(w http.ResponseWriter, r *http.Request) {
	// Routes: /api/agents/{name}/message
	path := r.URL.Path[len("/api/agents/"):]
	parts := splitPath(path)
	if len(parts) == 2 && parts[1] == "message" {
		srv.handleAgentMessage(w, r, parts[0])
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (srv *Server) handleAgentMessage(w http.ResponseWriter, r *http.Request, agentName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if srv.pool == nil {
		http.Error(w, "agent pool not available", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		Prompt string `json:"prompt"`
		ChatID int64  `json:"chat_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	response, err := srv.pool.SendMessageTo(r.Context(), agentName, body.ChatID, body.Prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("send message to %s: %v", agentName, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"response": response,
		"agent":    agentName,
	})
}

func (srv *Server) handleDelegateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if srv.pool == nil {
		http.Error(w, "agent pool not available", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		From         string `json:"from"`
		To           string `json:"to"`
		ProjectID    string `json:"project_id"`
		Title        string `json:"title"`
		Instructions string `json:"instructions"`
		Priority     int    `json:"priority"`
		ChatID       int64  `json:"chat_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.From == "" || body.To == "" || body.Title == "" || body.Instructions == "" {
		http.Error(w, "from, to, title, and instructions are required", http.StatusBadRequest)
		return
	}
	if body.Priority < 1 || body.Priority > 4 {
		body.Priority = 2
	}

	taskID := fmt.Sprintf("task-%s-%d", body.To, time.Now().UnixMilli())

	if err := srv.pool.DelegateTask(r.Context(), body.From, body.To, body.ChatID,
		body.ProjectID, taskID, body.Title, body.Instructions, body.Priority); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("delegate task: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"task_id": taskID,
		"status":  "delegated",
	})
}

func (srv *Server) handleTaskStatus(w http.ResponseWriter, _ *http.Request, id string) {
	t, err := srv.store.GetTask(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get task: %v", err)
		return
	}
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	assignedTo := ""
	if t.AssignedTo != nil {
		assignedTo = *t.AssignedTo
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"task_id":     t.ID,
		"status":      t.Status,
		"title":       t.Title,
		"assigned_to": assignedTo,
	})
}

// splitPath splits a URL path segment on "/" and filters empty parts.
func splitPath(path string) []string {
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

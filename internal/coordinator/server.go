package coordinator

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CJhuochai/project-brain/internal/indexer"
	"github.com/CJhuochai/project-brain/internal/report"
	"github.com/CJhuochai/project-brain/internal/service"
	"github.com/CJhuochai/project-brain/internal/storage"
	"github.com/CJhuochai/project-brain/internal/workspace"
)

type Server struct {
	root          string
	workspaceDir  string
	control       *storage.Control
	listener      net.Listener
	token         string
	release       func()
	done          chan struct{}
	once          sync.Once
	refreshMu     sync.Mutex
	feedbackMu    sync.Mutex
	refreshDone   chan struct{}
	refreshError  error
	refreshTarget string
	lastRefresh   indexer.RefreshResult
}

func Start(root string) (*Server, error) {
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		return nil, err
	}
	if err := storage.RemoveStaging(workspaceDir); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var release func()
	if os.Getenv("PB_COORDINATOR_LEASE_HELD") == "1" {
		release = func() { _ = os.Remove(filepath.Join(workspaceDir, "coordinator.lock")) }
	} else {
		release, err = acquireLease(workspaceDir)
		if err != nil {
			return nil, err
		}
	}
	control, err := storage.OpenControl(workspaceDir)
	if err != nil {
		release()
		return nil, err
	}
	if _, err := control.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = control.Close()
		release()
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		_ = control.Close()
		release()
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		_ = listener.Close()
		_ = control.Close()
		release()
		return nil, err
	}
	if _, err := control.Exec(`CREATE TABLE IF NOT EXISTS coordinator_state (id INTEGER PRIMARY KEY CHECK(id = 1), endpoint TEXT NOT NULL, token TEXT NOT NULL, pid INTEGER NOT NULL, protocol INTEGER NOT NULL, heartbeat TEXT NOT NULL)`); err != nil {
		_ = listener.Close()
		_ = control.Close()
		release()
		return nil, err
	}
	if _, err := control.Exec(`INSERT INTO coordinator_state(id, endpoint, token, pid, protocol, heartbeat) VALUES(1, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET endpoint=excluded.endpoint, token=excluded.token, pid=excluded.pid, protocol=excluded.protocol, heartbeat=excluded.heartbeat`, listener.Addr().String(), token, os.Getpid(), protocolVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		_ = listener.Close()
		_ = control.Close()
		release()
		return nil, err
	}
	server := &Server{root: root, workspaceDir: workspaceDir, control: control, listener: listener, token: token, release: release, done: make(chan struct{})}
	go server.accept()
	return server, nil
}

func Serve(root string) error {
	server, err := Start(root)
	if err != nil {
		return err
	}
	<-server.done
	return nil
}

func (server *Server) Close() error {
	var result error
	server.once.Do(func() {
		result = server.listener.Close()
		_ = server.control.Close()
		server.release()
		close(server.done)
	})
	return result
}

func (server *Server) accept() {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}
		go server.handle(connection)
	}
}

func (server *Server) handle(connection net.Conn) {
	defer connection.Close()
	host, _, err := net.SplitHostPort(connection.RemoteAddr().String())
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return
	}
	var request Request
	if err := json.NewDecoder(connection).Decode(&request); err != nil {
		return
	}
	if request.Token != server.token {
		_ = json.NewEncoder(connection).Encode(Response{Error: "unauthorized coordinator request"})
		return
	}
	response := server.execute(request)
	_ = json.NewEncoder(connection).Encode(response)
}

func (server *Server) execute(request Request) Response {
	if request.Operation == "health" {
		active, err := server.control.ActiveSnapshot()
		if err != nil {
			return Response{Error: err.Error()}
		}
		return Response{Result: json.RawMessage(`{"ok":true}`), Meta: server.metadata(active, "up_to_date", "")}
	}
	snapshot, meta, err := server.chooseSnapshot(request)
	if err != nil {
		return Response{Error: err.Error()}
	}
	if request.Operation == "workspace_status" {
		server.refreshMu.Lock()
		refresh := server.lastRefresh
		server.lastRefresh = indexer.RefreshResult{}
		server.refreshMu.Unlock()
		if refresh.IndexState == "" {
			index, err := storage.OpenSnapshot(snapshot.Path, false)
			if err != nil {
				return Response{Meta: meta, Error: err.Error()}
			}
			refresh, err = indexer.Status(server.root, index)
			_ = index.Close()
			if err != nil {
				return Response{Meta: meta, Error: err.Error()}
			}
		}
		encoded, err := json.Marshal(refresh)
		if err != nil {
			return Response{Meta: meta, Error: err.Error()}
		}
		return Response{Result: encoded, Meta: meta}
	}
	if request.Operation == "get_analysis_report" || request.Operation == "record_analysis_feedback" {
		if request.Operation == "record_analysis_feedback" {
			server.feedbackMu.Lock()
			defer server.feedbackMu.Unlock()
		}
		result, err := service.Execute(server.root, nil, server.control, request.Operation, request.Arguments, report.Provenance{})
		return server.result(result, meta, err)
	}
	index, err := storage.OpenSnapshot(snapshot.Path, false)
	if err != nil {
		return Response{Meta: meta, Error: err.Error()}
	}
	defer index.Close()
	result, err := service.Execute(server.root, index, server.control, request.Operation, request.Arguments, report.Provenance{SnapshotID: snapshot.ID, SnapshotCompleted: snapshot.Completed, Baselines: snapshot.Baseline, RuleRevision: meta.RuleRevision})
	return server.result(result, meta, err)
}

func (server *Server) result(result any, meta Metadata, err error) Response {
	if err != nil {
		return Response{Meta: meta, Error: err.Error()}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return Response{Meta: meta, Error: err.Error()}
	}
	return Response{Result: encoded, Meta: meta}
}

func (server *Server) chooseSnapshot(request Request) (storage.Snapshot, Metadata, error) {
	if request.SnapshotID != "" {
		snapshot, err := server.control.Snapshot(request.SnapshotID)
		if err != nil {
			return storage.Snapshot{}, Metadata{}, fmt.Errorf("snapshot not available: %s", request.SnapshotID)
		}
		return snapshot, server.metadata(snapshot, "up_to_date", ""), nil
	}
	active, err := server.control.ActiveSnapshot()
	if err != nil {
		return storage.Snapshot{}, Metadata{}, err
	}
	stale, target, err := server.baselineChanged(active)
	if err != nil || !stale {
		if err != nil {
			err = fmt.Errorf("check baseline: %w", err)
		}
		return active, server.metadata(active, "up_to_date", ""), err
	}
	done := server.refresh(target)
	if request.Freshness != "latest" {
		return active, server.metadata(active, "refreshing", target), nil
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		return active, server.metadata(active, "stale", target), nil
	}
	server.refreshMu.Lock()
	err = server.refreshError
	server.refreshMu.Unlock()
	if err != nil {
		return active, server.metadata(active, "stale", target), nil
	}
	active, err = server.control.ActiveSnapshot()
	if err != nil {
		return storage.Snapshot{}, Metadata{}, err
	}
	return active, server.metadata(active, "up_to_date", ""), nil
}

func (server *Server) metadata(snapshot storage.Snapshot, freshness, target string) Metadata {
	revision, err := server.control.RuleRevision()
	if err != nil {
		revision = 0
	}
	return Metadata{SnapshotID: snapshot.ID, RuleRevision: revision, Freshness: freshness, ActiveBaseline: snapshot.Baseline, RefreshTarget: target, RefreshTaskCount: 1}
}

func (server *Server) baselineChanged(active storage.Snapshot) (bool, string, error) {
	index, err := storage.OpenSnapshot(active.Path, false)
	if err != nil {
		return false, "", fmt.Errorf("open active snapshot: %w", err)
	}
	defer index.Close()
	repositories, err := workspace.Discover(server.root)
	if err != nil {
		return false, "", fmt.Errorf("discover baselines: %w", err)
	}
	var targets []string
	for _, repository := range repositories {
		if repository.BaselineState != workspace.BaselineKnown {
			continue
		}
		record, err := index.RepositoryRecord(repository.Path)
		if err != nil {
			return false, "", fmt.Errorf("read active repository %s: %w", repository.Name, err)
		}
		if record.BaselineCommit != repository.BaselineCommit {
			targets = append(targets, repository.Name+"@"+repository.BaselineCommit)
		}
	}
	return len(targets) > 0, strings.Join(targets, ","), nil
}

func (server *Server) refresh(target string) <-chan struct{} {
	server.refreshMu.Lock()
	defer server.refreshMu.Unlock()
	if server.refreshDone != nil {
		return server.refreshDone
	}
	server.refreshDone = make(chan struct{})
	server.refreshTarget = target
	server.refreshError = nil
	go func(done chan struct{}) {
		defer close(done)
		err := server.runRefresh()
		server.refreshMu.Lock()
		server.refreshError = err
		server.refreshDone = nil
		server.refreshTarget = ""
		server.refreshMu.Unlock()
	}(server.refreshDone)
	return server.refreshDone
}

func (server *Server) runRefresh() error {
	staging, discard, err := storage.PrepareStaging(server.control)
	if err != nil {
		return fmt.Errorf("prepare staging: %w", err)
	}
	defer discard()
	index, err := storage.OpenSnapshot(staging.Path, true)
	if err != nil {
		return fmt.Errorf("open staging: %w", err)
	}
	result, refreshErr := indexer.Refresh(server.root, index)
	closeErr := index.Close()
	if refreshErr != nil {
		return fmt.Errorf("refresh staging: %w", refreshErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close staging: %w", closeErr)
	}
	staging.Baseline = baselineText(result)
	if _, err := storage.ActivateStaging(server.control, staging); err != nil {
		return fmt.Errorf("activate staging: %w", err)
	}
	server.refreshMu.Lock()
	server.lastRefresh = result
	server.refreshMu.Unlock()
	return nil
}

func baselineText(result indexer.RefreshResult) string {
	var values []string
	for _, repository := range result.Repositories {
		if repository.BaselineCommit != "" {
			values = append(values, repository.Name+"@"+repository.BaselineCommit)
		}
	}
	return strings.Join(values, ",")
}

func acquireLease(workspaceDir string) (func(), error) {
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(workspaceDir, "coordinator.lock")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

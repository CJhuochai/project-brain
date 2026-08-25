package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/CJhuochai/project-brain/internal/storage"
)

type Client struct {
	endpoint string
	token    string
}

// Ensure returns the one healthy local coordinator for root. The first caller
// atomically leases startup; every other caller only waits for that process.
func Ensure(root, executable string) (*Client, error) {
	if client, err := healthyClient(root); err == nil {
		return client, nil
	}
	if strings.Contains(filepath.Base(executable), ".test") {
		if _, err := Start(root); err != nil {
			return nil, err
		}
		return healthyClient(root)
	}
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		return nil, err
	}
	release, err := acquireLease(workspaceDir)
	if err != nil && staleLease(workspaceDir) {
		lock := filepath.Join(workspaceDir, "coordinator.lock")
		_ = os.Rename(lock, lock+".stale."+fmt.Sprint(time.Now().UnixNano()))
		release, err = acquireLease(workspaceDir)
	}
	if err == nil {
		if executable == "" {
			executable, err = os.Executable()
			if err != nil {
				release()
				return nil, err
			}
		}
		command := exec.Command(executable, "coordinator", root)
		command.Env = append(os.Environ(), "PB_COORDINATOR_LEASE_HELD=1")
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Start(); err != nil {
			release()
			return nil, err
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if client, err := healthyClient(root); err == nil {
			return client, nil
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err == nil {
		release()
	}
	return nil, fmt.Errorf("coordinator did not become healthy for %s: %w", filepath.Clean(root), lastErr)
}

func staleLease(workspaceDir string) bool {
	info, err := os.Stat(filepath.Join(workspaceDir, "coordinator.lock"))
	return err == nil && time.Since(info.ModTime()) > 15*time.Second
}

func healthyClient(root string) (*Client, error) {
	client, err := Connect(root)
	if err != nil {
		return nil, err
	}
	context, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := client.Call(context, Request{Operation: "health"}); err != nil {
		return nil, err
	}
	return client, nil
}

func Connect(root string) (*Client, error) {
	workspaceDir, err := storage.WorkspaceDir(root)
	if err != nil {
		return nil, err
	}
	control, err := storage.OpenControlRead(workspaceDir)
	if err != nil {
		return nil, err
	}
	defer control.Close()
	client := &Client{}
	var protocol int
	if err := control.QueryRow(`SELECT endpoint, token, protocol FROM coordinator_state WHERE id = 1`).Scan(&client.endpoint, &client.token, &protocol); err != nil {
		return nil, err
	}
	if protocol != protocolVersion {
		return nil, fmt.Errorf("unsupported coordinator protocol: %d", protocol)
	}
	return client, nil
}

func (client *Client) Call(ctx context.Context, request Request) (Response, error) {
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", client.endpoint)
	if err != nil {
		return Response{}, err
	}
	defer connection.Close()
	request.Token = client.token
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	}
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return Response{}, err
	}
	var response Response
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return Response{}, err
	}
	if response.Error != "" {
		return response, fmt.Errorf("coordinator: %s", response.Error)
	}
	return response, nil
}

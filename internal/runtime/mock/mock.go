// Package mock provides a mock implementation of the runtime.Runtime interface for testing.
package mock

import (
	"context"
	"sync"
	"time"

	"github.com/ajaxzhan/sandbox-rls/internal/runtime"
	"github.com/ajaxzhan/sandbox-rls/pkg/types"
)

// MockRuntime is a mock implementation of runtime.RuntimeWithExecutor for testing.
type MockRuntime struct {
	mu       sync.RWMutex
	sandboxes map[string]*types.Sandbox
	
	// Hooks for customizing behavior in tests
	OnCreate  func(ctx context.Context, config *runtime.SandboxConfig) (*types.Sandbox, error)
	OnStart   func(ctx context.Context, sandboxID string) error
	OnStop    func(ctx context.Context, sandboxID string) error
	OnDestroy func(ctx context.Context, sandboxID string) error
	OnExec    func(ctx context.Context, sandboxID string, req *types.ExecRequest) (*types.ExecResult, error)
}

// New creates a new MockRuntime.
func New() *MockRuntime {
	return &MockRuntime{
		sandboxes: make(map[string]*types.Sandbox),
	}
}

// Name returns the name of this runtime implementation.
func (m *MockRuntime) Name() string {
	return "mock"
}

// Create creates a new sandbox but does not start it.
func (m *MockRuntime) Create(ctx context.Context, config *runtime.SandboxConfig) (*types.Sandbox, error) {
	if m.OnCreate != nil {
		return m.OnCreate(ctx, config)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sandbox := &types.Sandbox{
		ID:          config.ID,
		CodebaseID:  config.CodebaseID,
		Permissions: config.Permissions,
		Status:      types.StatusPending,
		Labels:      config.Labels,
		CreatedAt:   time.Now(),
		MountPoint:  config.MountPoint,
	}

	m.sandboxes[config.ID] = sandbox
	return sandbox, nil
}

// Start starts a previously created sandbox.
func (m *MockRuntime) Start(ctx context.Context, sandboxID string) error {
	if m.OnStart != nil {
		return m.OnStart(ctx, sandboxID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sandbox, ok := m.sandboxes[sandboxID]
	if !ok {
		return types.ErrSandboxNotFound
	}

	if sandbox.Status == types.StatusRunning {
		return types.ErrAlreadyRunning
	}

	sandbox.Status = types.StatusRunning
	now := time.Now()
	sandbox.StartedAt = &now
	return nil
}

// Stop stops a running sandbox without destroying it.
func (m *MockRuntime) Stop(ctx context.Context, sandboxID string) error {
	if m.OnStop != nil {
		return m.OnStop(ctx, sandboxID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sandbox, ok := m.sandboxes[sandboxID]
	if !ok {
		return types.ErrSandboxNotFound
	}

	if sandbox.Status != types.StatusRunning {
		return types.ErrNotRunning
	}

	sandbox.Status = types.StatusStopped
	now := time.Now()
	sandbox.StoppedAt = &now
	return nil
}

// Destroy destroys a sandbox, releasing all resources.
func (m *MockRuntime) Destroy(ctx context.Context, sandboxID string) error {
	if m.OnDestroy != nil {
		return m.OnDestroy(ctx, sandboxID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sandboxes[sandboxID]; !ok {
		return types.ErrSandboxNotFound
	}

	delete(m.sandboxes, sandboxID)
	return nil
}

// Get retrieves information about a sandbox.
func (m *MockRuntime) Get(ctx context.Context, sandboxID string) (*types.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sandbox, ok := m.sandboxes[sandboxID]
	if !ok {
		return nil, types.ErrSandboxNotFound
	}

	// Return a copy to prevent external modifications
	copy := *sandbox
	return &copy, nil
}

// List returns all sandboxes managed by this runtime.
func (m *MockRuntime) List(ctx context.Context) ([]*types.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*types.Sandbox, 0, len(m.sandboxes))
	for _, sb := range m.sandboxes {
		copy := *sb
		result = append(result, &copy)
	}
	return result, nil
}

// Exec executes a command in the sandbox and returns the result.
func (m *MockRuntime) Exec(ctx context.Context, sandboxID string, req *types.ExecRequest) (*types.ExecResult, error) {
	if m.OnExec != nil {
		return m.OnExec(ctx, sandboxID, req)
	}

	m.mu.RLock()
	sandbox, ok := m.sandboxes[sandboxID]
	m.mu.RUnlock()

	if !ok {
		return nil, types.ErrSandboxNotFound
	}

	if sandbox.Status != types.StatusRunning {
		return nil, types.ErrNotRunning
	}

	// Default mock behavior: return empty successful result
	return &types.ExecResult{
		Stdout:   "",
		Stderr:   "",
		ExitCode: 0,
		Duration: time.Millisecond,
	}, nil
}

// ExecStream executes a command and streams output.
func (m *MockRuntime) ExecStream(ctx context.Context, sandboxID string, req *types.ExecRequest, output chan<- []byte) error {
	m.mu.RLock()
	sandbox, ok := m.sandboxes[sandboxID]
	m.mu.RUnlock()

	if !ok {
		return types.ErrSandboxNotFound
	}

	if sandbox.Status != types.StatusRunning {
		return types.ErrNotRunning
	}

	// Default mock behavior: just close the channel
	close(output)
	return nil
}

// Verify interface compliance at compile time
var _ runtime.RuntimeWithExecutor = (*MockRuntime)(nil)

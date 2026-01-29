// Package bwrap provides a sandbox runtime implementation using bubblewrap (bwrap).
// On Linux, it uses bwrap for actual process isolation.
// On other systems (macOS, Windows), it falls back to a local process executor for development/testing.
package bwrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	rt "github.com/ajaxzhan/sandbox-rls/internal/runtime"
	"github.com/ajaxzhan/sandbox-rls/pkg/types"
)

// Config holds configuration for the BwrapRuntime.
type Config struct {
	// BwrapPath is the path to the bwrap binary (default: "bwrap")
	BwrapPath string

	// DefaultTimeout is the default timeout for operations
	DefaultTimeout time.Duration

	// WorkDir is the base directory for sandbox working directories
	WorkDir string

	// EnableNetworking allows network access in sandboxes
	EnableNetworking bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		BwrapPath:        "bwrap",
		DefaultTimeout:   30 * time.Second,
		WorkDir:          "/tmp/sandbox-rls",
		EnableNetworking: false,
	}
}

// sandboxState holds internal state for a sandbox.
type sandboxState struct {
	sandbox *types.Sandbox
	config  *rt.SandboxConfig
	cmd     *exec.Cmd // The running process (if any)
	cancel  context.CancelFunc
}

// BwrapRuntime implements runtime.RuntimeWithExecutor using bubblewrap.
type BwrapRuntime struct {
	mu       sync.RWMutex
	config   *Config
	states   map[string]*sandboxState
	isLinux  bool
}

// New creates a new BwrapRuntime with the given configuration.
func New(config *Config) *BwrapRuntime {
	if config == nil {
		config = DefaultConfig()
	}

	return &BwrapRuntime{
		config:  config,
		states:  make(map[string]*sandboxState),
		isLinux: runtime.GOOS == "linux",
	}
}

// Name returns the name of this runtime implementation.
func (r *BwrapRuntime) Name() string {
	if r.isLinux {
		return "bwrap"
	}
	return "bwrap-compat" // Compatibility mode for non-Linux
}

// Create creates a new sandbox but does not start it.
func (r *BwrapRuntime) Create(ctx context.Context, config *rt.SandboxConfig) (*types.Sandbox, error) {
	if config.ID == "" {
		return nil, fmt.Errorf("sandbox ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if sandbox already exists
	if _, exists := r.states[config.ID]; exists {
		return nil, fmt.Errorf("sandbox %s already exists", config.ID)
	}

	// Validate codebase path exists
	if config.CodebasePath != "" {
		if _, err := os.Stat(config.CodebasePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("codebase path does not exist: %s", config.CodebasePath)
		}
	}

	sandbox := &types.Sandbox{
		ID:          config.ID,
		CodebaseID:  config.CodebaseID,
		Permissions: config.Permissions,
		Status:      types.StatusPending,
		Labels:      config.Labels,
		CreatedAt:   time.Now(),
		MountPoint:  config.MountPoint,
	}

	r.states[config.ID] = &sandboxState{
		sandbox: sandbox,
		config:  config,
	}

	return sandbox, nil
}

// Start starts a previously created sandbox.
func (r *BwrapRuntime) Start(ctx context.Context, sandboxID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.states[sandboxID]
	if !ok {
		return types.ErrSandboxNotFound
	}

	if state.sandbox.Status == types.StatusRunning {
		return types.ErrAlreadyRunning
	}

	// On Linux, we would start a long-running bwrap process here.
	// For now, we just mark it as running since exec will spawn processes as needed.
	// In a full implementation, we might keep a shell process alive in the sandbox.

	state.sandbox.Status = types.StatusRunning
	now := time.Now()
	state.sandbox.StartedAt = &now

	return nil
}

// Stop stops a running sandbox without destroying it.
func (r *BwrapRuntime) Stop(ctx context.Context, sandboxID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.states[sandboxID]
	if !ok {
		return types.ErrSandboxNotFound
	}

	if state.sandbox.Status != types.StatusRunning {
		return types.ErrNotRunning
	}

	// Cancel any running processes
	if state.cancel != nil {
		state.cancel()
	}

	// Kill the process if it's still running
	if state.cmd != nil && state.cmd.Process != nil {
		_ = state.cmd.Process.Kill()
		state.cmd = nil
	}

	state.sandbox.Status = types.StatusStopped
	now := time.Now()
	state.sandbox.StoppedAt = &now

	return nil
}

// Destroy destroys a sandbox, releasing all resources.
func (r *BwrapRuntime) Destroy(ctx context.Context, sandboxID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.states[sandboxID]
	if !ok {
		return types.ErrSandboxNotFound
	}

	// Stop any running processes first
	if state.cancel != nil {
		state.cancel()
	}
	if state.cmd != nil && state.cmd.Process != nil {
		_ = state.cmd.Process.Kill()
	}

	delete(r.states, sandboxID)
	return nil
}

// Get retrieves information about a sandbox.
func (r *BwrapRuntime) Get(ctx context.Context, sandboxID string) (*types.Sandbox, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, ok := r.states[sandboxID]
	if !ok {
		return nil, types.ErrSandboxNotFound
	}

	// Return a copy
	sandbox := *state.sandbox
	return &sandbox, nil
}

// List returns all sandboxes managed by this runtime.
func (r *BwrapRuntime) List(ctx context.Context) ([]*types.Sandbox, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*types.Sandbox, 0, len(r.states))
	for _, state := range r.states {
		sandbox := *state.sandbox
		result = append(result, &sandbox)
	}
	return result, nil
}

// Exec executes a command in the sandbox and returns the result.
func (r *BwrapRuntime) Exec(ctx context.Context, sandboxID string, req *types.ExecRequest) (*types.ExecResult, error) {
	r.mu.RLock()
	state, ok := r.states[sandboxID]
	if !ok {
		r.mu.RUnlock()
		return nil, types.ErrSandboxNotFound
	}

	if state.sandbox.Status != types.StatusRunning {
		r.mu.RUnlock()
		return nil, types.ErrNotRunning
	}

	config := state.config
	r.mu.RUnlock()

	// Set timeout if specified
	timeout := req.Timeout
	if timeout == 0 {
		timeout = r.config.DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	var cmd *exec.Cmd
	if r.isLinux {
		cmd = r.buildBwrapCommand(ctx, config, req)
	} else {
		// Compatibility mode: run command directly (no isolation)
		cmd = r.buildLocalCommand(ctx, config, req)
	}

	// Capture output
	stdout, err := cmd.Output()
	duration := time.Since(start)

	result := &types.ExecResult{
		Duration: duration,
	}

	if err != nil {
		// Check for timeout first - context deadline exceeded takes priority
		if ctx.Err() == context.DeadlineExceeded {
			return nil, types.ErrTimeout
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Stderr = string(exitErr.Stderr)
			result.Stdout = string(stdout)
			// Non-zero exit is not an error from our perspective
			return result, nil
		}

		return nil, fmt.Errorf("exec failed: %w", err)
	}

	result.Stdout = string(stdout)
	result.ExitCode = 0
	return result, nil
}

// ExecStream executes a command and streams output.
func (r *BwrapRuntime) ExecStream(ctx context.Context, sandboxID string, req *types.ExecRequest, output chan<- []byte) error {
	r.mu.RLock()
	state, ok := r.states[sandboxID]
	if !ok {
		r.mu.RUnlock()
		close(output)
		return types.ErrSandboxNotFound
	}

	if state.sandbox.Status != types.StatusRunning {
		r.mu.RUnlock()
		close(output)
		return types.ErrNotRunning
	}

	config := state.config
	r.mu.RUnlock()

	// Set timeout if specified
	timeout := req.Timeout
	if timeout == 0 {
		timeout = r.config.DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if r.isLinux {
		cmd = r.buildBwrapCommand(ctx, config, req)
	} else {
		cmd = r.buildLocalCommand(ctx, config, req)
	}

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		close(output)
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		close(output)
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Stream output
	go func() {
		defer close(output)
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case output <- data:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	return cmd.Wait()
}

// buildBwrapCommand builds a bwrap command for Linux.
func (r *BwrapRuntime) buildBwrapCommand(ctx context.Context, config *rt.SandboxConfig, req *types.ExecRequest) *exec.Cmd {
	args := []string{
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/lib", "/lib",
		"--ro-bind", "/lib64", "/lib64",
		"--ro-bind", "/bin", "/bin",
		"--ro-bind", "/sbin", "/sbin",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--unshare-pid",
		"--unshare-uts",
		"--unshare-ipc",
		"--die-with-parent",
	}

	// Network isolation
	if !r.config.EnableNetworking {
		args = append(args, "--unshare-net")
	}

	// Bind the codebase
	if config.CodebasePath != "" {
		workdir := "/workspace"
		if config.MountPoint != "" {
			workdir = config.MountPoint
		}
		args = append(args, "--bind", config.CodebasePath, workdir)
		args = append(args, "--chdir", workdir)
	}

	// Add the shell command
	args = append(args, "/bin/sh", "-c", req.Command)

	cmd := exec.CommandContext(ctx, r.config.BwrapPath, args...)

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up stdin if provided
	if req.Stdin != "" {
		cmd.Stdin = nil // We'll handle this differently if needed
	}

	return cmd
}

// buildLocalCommand builds a local command for non-Linux systems (development mode).
func (r *BwrapRuntime) buildLocalCommand(ctx context.Context, config *rt.SandboxConfig, req *types.ExecRequest) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", req.Command)

	// Set working directory
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	} else if config.CodebasePath != "" {
		cmd.Dir = config.CodebasePath
	}

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up process group for cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return cmd
}

// IsBwrapAvailable checks if bwrap is available on the system.
func IsBwrapAvailable() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("bwrap")
	return err == nil
}

// Verify interface compliance at compile time
var _ rt.RuntimeWithExecutor = (*BwrapRuntime)(nil)

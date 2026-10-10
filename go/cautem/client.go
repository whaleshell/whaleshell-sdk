// Package cautem is the Go SDK for the cautem gateway control plane.
//
// RPC methods authenticate with a bearer token (OIDC access token or the
// local-dev token from `<gateway data dir>/auth_token`). New picks up
// CAUTEM_GATEWAY_TOKEN; NewWithToken sets it explicitly. Resource facades
// use OpenShell or cautem RPC. Health and local auth bootstrap remain HTTP;
// CLI-managed sandbox registry operations use a dedicated Control RPC surface.
//
// Exec runs over the pinned OpenShell ExecSandbox gRPC method.
// Interactive sessions and IDE access use SSH sessions: CreateSSHSession plus
// `cautem ssh-proxy` / `cautem sandbox connect`.
package cautem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	openshell "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	gc "github.com/cautem/cautem-sdk/internal/gatewayclient"
	"google.golang.org/grpc"
)

// EnvToken is the default bearer for New.
const EnvToken = "CAUTEM_GATEWAY_TOKEN"

// ErrConnectUnsupported means interactive sessions stay on the CLI.
var ErrConnectUnsupported = errors.New("cautem-sdk: interactive connect is not supported; use: cautem sandbox connect <name>")

// ErrSandboxNotReady is returned when the sandbox supervisor relay is not connected.
var ErrSandboxNotReady = gc.ErrSandboxNotReady

// Client exposes curated OpenShell and cautem RPC methods plus explicit bootstrap HTTP calls.
type Client struct {
	http  *gc.Client
	base  string
	token string
	// Workspace selects the default workspace for resource-oriented methods.
	Workspace     string
	rpcMu         sync.Mutex
	rpcConn       *grpc.ClientConn
	openShellConn *grpc.ClientConn
	upstream      *openshell.Client
}

func (c *Client) workspace() string {
	if c.Workspace != "" {
		return c.Workspace
	}
	return defaultWorkspace
}

// New builds a client for a gateway base URL (e.g. http://127.0.0.1:7443),
// authenticated with $CAUTEM_GATEWAY_TOKEN when set.
func New(baseURL string) *Client {
	return NewWithToken(baseURL, os.Getenv(EnvToken))
}

// NewWithToken returns a client with bearer auth.
func NewWithToken(base, token string) *Client {
	token = strings.TrimSpace(token)
	return &Client{http: gc.NewWithToken(base, token), base: strings.TrimRight(base, "/"), token: token}
}

// Close releases the native gRPC connection opened for RPC-backed methods.
func (c *Client) Close() error {
	c.rpcMu.Lock()
	defer c.rpcMu.Unlock()
	var closeErr error
	if c.upstream != nil {
		closeErr = errors.Join(closeErr, c.upstream.Close())
		c.upstream = nil
	}
	if c.openShellConn != nil {
		closeErr = errors.Join(closeErr, c.openShellConn.Close())
		c.openShellConn = nil
	}
	if c.rpcConn != nil {
		closeErr = errors.Join(closeErr, c.rpcConn.Close())
		c.rpcConn = nil
	}
	return closeErr
}

// Stable type aliases for cautem gateway resources.
type (
	Labels                = gc.Labels
	Credentials           = gc.Credentials
	ProviderConfig        = gc.ProviderConfig
	RefreshMaterial       = gc.RefreshMaterial
	CredentialBindings    = gc.CredentialBindings
	CredentialExpiry      = gc.CredentialExpiry
	Sandbox               = gc.Sandbox
	ExecResult            = gc.ExecResult
	LogLine               = gc.LogLine
	Proposal              = gc.Proposal
	ProviderRecord        = gc.ProviderRecord
	ProviderRefreshConfig = gc.ProviderRefreshConfig
	PolicyRevisionMeta    = gc.PolicyRevisionMeta
	InferenceRoute        = gc.InferenceRoute
	ServiceRecord         = gc.ServiceRecord
	SSHSession            = gc.SSHSession
	SSHSessionInfo        = gc.SSHSessionInfo
)

// Create creates a runtime sandbox through the authorized Control RPC.
func (c *Client) Create(ctx context.Context, sb Sandbox) error {
	_, err := c.CreateControlSandbox(ctx, sb, nil)
	return err
}

// List returns caller-visible sandboxes through the allowlisted control API.
func (c *Client) List(ctx context.Context) ([]Sandbox, error) {
	return c.ListControlSandboxes(ctx, "", true)
}

// Get returns one redacted sandbox summary by name in the default workspace.
func (c *Client) Get(ctx context.Context, name string) (Sandbox, error) {
	return c.GetControlSandbox(ctx, "default", name)
}

// Delete removes a runtime sandbox using its current resource version.
func (c *Client) Delete(ctx context.Context, name string) error {
	return c.DeleteControlSandbox(ctx, "default", name)
}

// Start starts a runtime sandbox in the default workspace.
func (c *Client) Start(ctx context.Context, name string) (Sandbox, error) {
	return c.StartControlSandbox(ctx, "default", name)
}

// Stop stops a runtime sandbox in the default workspace.
func (c *Client) Stop(ctx context.Context, name string) (Sandbox, error) {
	return c.StopControlSandbox(ctx, "default", name)
}

// Exec runs argv in the sandbox using the pinned OpenShell RPC client.
func (c *Client) Exec(ctx context.Context, name string, argv ...string) (ExecResult, error) {
	if name == "" || len(argv) == 0 {
		return ExecResult{}, fmt.Errorf("usage: Exec(name, argv...)")
	}
	client, err := c.openShellClient()
	if err != nil {
		return ExecResult{}, err
	}
	result, err := client.Exec().Run(ctx, "default", name, argv)
	if err != nil {
		return ExecResult{}, fmt.Errorf("execute in sandbox %q: %w", name, err)
	}
	return ExecResult{ExitCode: result.ExitCode, Output: string(result.Stdout) + string(result.Stderr)}, nil
}

// Connect is intentionally unsupported in the SDK.
func (c *Client) Connect(_ context.Context, _ string) error {
	return ErrConnectUnsupported
}

// Healthz is the unauthenticated HTTP readiness probe used during CLI bootstrap.
func (c *Client) Healthz(ctx context.Context) (map[string]any, error) { return c.http.Healthz(ctx) }

// Info returns authorized gateway diagnostics over Control RPC.
func (c *Client) Info(ctx context.Context) (map[string]any, error) { return c.gatewayInfo(ctx) }

// AuthLogin performs the local-dev HTTP token bootstrap flow.
func (c *Client) AuthLogin(ctx context.Context) (string, error) { return c.http.AuthLogin(ctx) }

// UpsertSandbox synchronizes metadata for a CLI-managed external runtime over RPC.
func (c *Client) UpsertSandbox(ctx context.Context, sandbox Sandbox) error {
	return c.SyncManagedSandbox(ctx, sandbox)
}

// DeleteSandbox removes the registry entry for a CLI-managed runtime.
func (c *Client) DeleteSandbox(ctx context.Context, name string) error {
	return c.DeleteManagedSandbox(ctx, name)
}

// ListSandboxes returns the gateway registry records required by CLI rules.
func (c *Client) ListSandboxes(ctx context.Context) ([]Sandbox, error) {
	return c.ListControlSandboxes(ctx, c.workspace(), false)
}

// GetSandbox returns metadata for one CLI-managed sandbox.
func (c *Client) GetSandbox(ctx context.Context, name string) (Sandbox, error) {
	return c.GetManagedSandbox(ctx, name)
}

// IssueSandboxToken provisions a supervisor credential for a CLI-managed sandbox.
func (c *Client) IssueSandboxToken(ctx context.Context, name string) (string, error) {
	return c.IssueManagedSandboxToken(ctx, name)
}

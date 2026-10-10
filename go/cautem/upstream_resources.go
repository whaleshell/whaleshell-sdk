package cautem

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	openshell "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	opentypes "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	gc "github.com/cautem/cautem-sdk/internal/gatewayclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultWorkspace = "default"
const runtimeCredentialsAnnotation = "cautem.io/runtime-credentials"

// ListProviders lists provider metadata through the pinned OpenShell contract.
// Credential values are never copied into the public SDK record.
func (c *Client) ListProviders(ctx context.Context) ([]ProviderRecord, error) {
	client, err := c.openShellClient()
	if err != nil {
		return nil, err
	}
	providers, err := client.Providers().List(ctx, c.workspace(), openshell.ListOptions{Limit: 1000})
	if err != nil {
		return nil, fmt.Errorf("list OpenShell providers: %w", err)
	}
	result := make([]ProviderRecord, 0, len(providers))
	for _, provider := range providers {
		result = append(result, providerRecordFromOpenShell(provider))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// GetProvider reads provider metadata through the pinned OpenShell contract.
func (c *Client) GetProvider(ctx context.Context, name string) (ProviderRecord, error) {
	client, err := c.openShellClient()
	if err != nil {
		return ProviderRecord{}, err
	}
	provider, err := client.Providers().Get(ctx, c.workspace(), name)
	if err != nil {
		return ProviderRecord{}, fmt.Errorf("get OpenShell provider %q: %w", name, err)
	}
	return providerRecordFromOpenShell(provider), nil
}

// PutProvider creates or updates a provider through the pinned OpenShell RPC.
func (c *Client) PutProvider(ctx context.Context, record ProviderRecord) error {
	if record.Name == "" || record.Type == "" {
		return fmt.Errorf("provider name and type are required")
	}
	// OpenShell Provider.Update replaces the credential map. Use the narrow
	// cautem RPC for a partial patch so old secret values never need to be
	// read or sent back by the client.
	if !sameCredentialKeys(record.EnvVars, record.Credentials) {
		return c.UpdateProviderCredentials(ctx, record.Workspace, record.Name, record.Credentials)
	}
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	workspace := record.Workspace
	if workspace == "" {
		workspace = c.workspace()
	}
	provider := &openshell.Provider{
		Name:      record.Name,
		Type:      record.Type,
		Workspace: workspace,
		Annotations: map[string]string{
			runtimeCredentialsAnnotation: strconv.FormatBool(record.RuntimeCredentials),
		},
		Spec: opentypes.ProviderSpec{
			Credentials:         map[string]string(record.Credentials),
			Config:              map[string]string(record.Config),
			CredentialExpiresAt: make(map[string]time.Time, len(record.CredentialExpiresAtMS)),
		},
	}
	for key, expiresAtMS := range record.CredentialExpiresAtMS {
		provider.Spec.CredentialExpiresAt[key] = time.UnixMilli(expiresAtMS)
	}
	current, getErr := client.Providers().Get(ctx, workspace, record.Name)
	if getErr == nil {
		provider.ID = current.ID
		provider.ResourceVersion = current.ResourceVersion
		_, err = client.Providers().Update(ctx, workspace, provider)
	} else if status.Code(getErr) == codes.NotFound {
		_, err = client.Providers().Create(ctx, workspace, provider)
	} else {
		return fmt.Errorf("inspect OpenShell provider %q before update: %w", record.Name, getErr)
	}
	if err != nil {
		return fmt.Errorf("write OpenShell provider %q: %w", record.Name, err)
	}
	for key, refresh := range record.Refresh {
		strategy, ok := openShellRefreshStrategy(refresh.Strategy)
		if !ok {
			return fmt.Errorf("provider %q credential %q: unsupported refresh strategy %q", record.Name, key, refresh.Strategy)
		}
		config := &openshell.RefreshConfig{
			Provider: record.Name, CredentialKey: key, Strategy: strategy,
			Material: refresh.Material,
		}
		if refresh.ExpiresAtMS > 0 {
			expires := time.UnixMilli(refresh.ExpiresAtMS)
			config.ExpiresAt = &expires
		}
		if _, err := client.Providers().Refresh().Configure(ctx, workspace, config); err != nil {
			return fmt.Errorf("configure OpenShell refresh for provider %q credential %q: %w", record.Name, key, err)
		}
	}
	return nil
}

func sameCredentialKeys(keys []string, credentials map[string]string) bool {
	if len(keys) != len(credentials) {
		return false
	}
	for _, key := range keys {
		if _, ok := credentials[key]; !ok {
			return false
		}
	}
	return true
}

// DeleteProvider deletes a provider through the pinned OpenShell contract.
func (c *Client) DeleteProvider(ctx context.Context, name string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	if err := client.Providers().Delete(ctx, c.workspace(), name); err != nil {
		return fmt.Errorf("delete OpenShell provider %q: %w", name, err)
	}
	return nil
}

// AttachProvider attaches a provider with an optimistic sandbox version check.
func (c *Client) AttachProvider(ctx context.Context, sandbox, provider string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	current, err := client.Sandboxes().Get(ctx, c.workspace(), sandbox)
	if err != nil {
		return fmt.Errorf("get OpenShell sandbox %q before provider attach: %w", sandbox, err)
	}
	if _, err := client.Sandboxes().AttachProvider(ctx, c.workspace(), sandbox, provider, current.ResourceVersion); err != nil {
		return fmt.Errorf("attach OpenShell provider %q to sandbox %q: %w", provider, sandbox, err)
	}
	return nil
}

// DetachProvider detaches a provider with an optimistic sandbox version check.
func (c *Client) DetachProvider(ctx context.Context, sandbox, provider string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	current, err := client.Sandboxes().Get(ctx, c.workspace(), sandbox)
	if err != nil {
		return fmt.Errorf("get OpenShell sandbox %q before provider detach: %w", sandbox, err)
	}
	if _, err := client.Sandboxes().DetachProvider(ctx, c.workspace(), sandbox, provider, current.ResourceVersion); err != nil {
		return fmt.Errorf("detach OpenShell provider %q from sandbox %q: %w", provider, sandbox, err)
	}
	return nil
}

// ListSandboxProviders lists attached provider metadata without credential values.
func (c *Client) ListSandboxProviders(ctx context.Context, sandbox string) ([]gc.SandboxProviderAttachment, error) {
	client, err := c.openShellClient()
	if err != nil {
		return nil, err
	}
	providers, err := client.Sandboxes().ListProviders(ctx, c.workspace(), sandbox)
	if err != nil {
		return nil, fmt.Errorf("list OpenShell providers attached to sandbox %q: %w", sandbox, err)
	}
	result := make([]gc.SandboxProviderAttachment, 0, len(providers))
	for _, provider := range providers {
		record := providerRecordFromOpenShell(provider)
		result = append(result, gc.SandboxProviderAttachment{Name: record.Name, Type: record.Type, EnvVars: record.EnvVars})
	}
	return result, nil
}

// CreateSSHSession issues a short-lived session over the OpenShell RPC.
func (c *Client) CreateSSHSession(ctx context.Context, sandbox string) (SSHSession, error) {
	client, err := c.openShellClient()
	if err != nil {
		return SSHSession{}, err
	}
	resource, err := client.Sandboxes().Get(ctx, c.workspace(), sandbox)
	if err != nil {
		return SSHSession{}, fmt.Errorf("get OpenShell sandbox %q for SSH: %w", sandbox, err)
	}
	session, err := client.SSH().CreateSession(ctx, c.workspace(), resource.ID)
	if err != nil {
		var upstreamErr *opentypes.StatusError
		if errors.As(err, &upstreamErr) && upstreamErr.Code == opentypes.ErrorConflict && strings.Contains(strings.ToLower(upstreamErr.Message), "not ready") {
			return SSHSession{}, fmt.Errorf("%w: %s", ErrSandboxNotReady, upstreamErr.Message)
		}
		return SSHSession{}, fmt.Errorf("create OpenShell SSH session for sandbox %q: %w", sandbox, err)
	}
	return SSHSession{
		SandboxID: session.SandboxID, Token: session.Token,
		GatewayHost: session.GatewayHost, GatewayPort: int(session.GatewayPort),
		GatewayScheme: session.GatewayScheme, ExpiresAtMS: session.ExpiresAtMs,
	}, nil
}

// RevokeSSHSession revokes a session by its secret token using OpenShell RPC.
func (c *Client) RevokeSSHSession(ctx context.Context, token string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	if _, err := client.SSH().RevokeSession(ctx, c.workspace(), token); err != nil {
		return fmt.Errorf("revoke OpenShell SSH session: %w", err)
	}
	return nil
}

// ConfigureProviderRefresh configures gateway-managed credential rotation via OpenShell RPC.
func (c *Client) ConfigureProviderRefresh(ctx context.Context, name, key, strategy string, material map[string]string, expiresAtMS int64) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	openStrategy, ok := openShellRefreshStrategy(strategy)
	if !ok {
		return fmt.Errorf("unsupported refresh strategy %q", strategy)
	}
	config := &openshell.RefreshConfig{Provider: name, CredentialKey: key, Strategy: openStrategy, Material: material}
	if expiresAtMS > 0 {
		expires := time.UnixMilli(expiresAtMS)
		config.ExpiresAt = &expires
	}
	if _, err := client.Providers().Refresh().Configure(ctx, c.workspace(), config); err != nil {
		return fmt.Errorf("configure OpenShell provider refresh: %w", err)
	}
	return nil
}

// RotateProviderRefresh rotates one provider credential through OpenShell RPC.
func (c *Client) RotateProviderRefresh(ctx context.Context, name, key string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	if _, err := client.Providers().Refresh().Rotate(ctx, c.workspace(), name, key); err != nil {
		return fmt.Errorf("rotate OpenShell provider credential: %w", err)
	}
	return nil
}

// DeleteProviderRefresh removes refresh configuration through OpenShell RPC.
func (c *Client) DeleteProviderRefresh(ctx context.Context, name, key string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	if _, err := client.Providers().Refresh().Delete(ctx, c.workspace(), name, key); err != nil {
		return fmt.Errorf("delete OpenShell provider refresh: %w", err)
	}
	return nil
}

// ListWorkspaces lists workspaces through the pinned OpenShell contract.
func (c *Client) ListWorkspaces(ctx context.Context) ([]gc.WorkspaceRecord, error) {
	client, err := c.openShellClient()
	if err != nil {
		return nil, err
	}
	items, err := client.Workspaces().List(ctx, openshell.ListOptions{Limit: 1000})
	if err != nil {
		return nil, fmt.Errorf("list OpenShell workspaces: %w", err)
	}
	result := make([]gc.WorkspaceRecord, 0, len(items))
	for _, workspace := range items {
		result = append(result, gc.WorkspaceRecord{Name: workspace.Name})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// GetWorkspace reads one workspace and its members through OpenShell RPC.
func (c *Client) GetWorkspace(ctx context.Context, name string) (gc.WorkspaceRecord, error) {
	client, err := c.openShellClient()
	if err != nil {
		return gc.WorkspaceRecord{}, err
	}
	if _, err = client.Workspaces().Get(ctx, name); err != nil {
		return gc.WorkspaceRecord{}, fmt.Errorf("get OpenShell workspace %q: %w", name, err)
	}
	members, err := client.Workspaces().ListMembers(ctx, name, openshell.ListOptions{Limit: 1000})
	if err != nil {
		return gc.WorkspaceRecord{}, fmt.Errorf("list OpenShell workspace members %q: %w", name, err)
	}
	result := gc.WorkspaceRecord{Name: name, Members: make([]gc.WorkspaceMember, 0, len(members))}
	for _, member := range members {
		result.Members = append(result.Members, gc.WorkspaceMember{Subject: member.PrincipalSubject, Role: strings.ToLower(string(member.Role))})
	}
	return result, nil
}

// CreateWorkspace creates a workspace through the pinned OpenShell contract.
func (c *Client) CreateWorkspace(ctx context.Context, name string) (gc.WorkspaceRecord, error) {
	client, err := c.openShellClient()
	if err != nil {
		return gc.WorkspaceRecord{}, err
	}
	if _, err := client.Workspaces().Create(ctx, name, nil); err != nil {
		return gc.WorkspaceRecord{}, fmt.Errorf("create OpenShell workspace %q: %w", name, err)
	}
	return gc.WorkspaceRecord{Name: name}, nil
}

// DeleteWorkspace deletes a workspace through the pinned OpenShell contract.
func (c *Client) DeleteWorkspace(ctx context.Context, name string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	if err := client.Workspaces().Delete(ctx, name); err != nil {
		return fmt.Errorf("delete OpenShell workspace %q: %w", name, err)
	}
	return nil
}

// WorkspaceMemberAdd adds or updates one workspace member through OpenShell RPC.
func (c *Client) WorkspaceMemberAdd(ctx context.Context, name, subject, role string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	openRole := openshell.WorkspaceRoleUser
	if strings.EqualFold(role, "admin") {
		openRole = openshell.WorkspaceRoleAdmin
	}
	if _, err := client.Workspaces().AddMember(ctx, name, subject, openRole); err != nil {
		return fmt.Errorf("add OpenShell workspace member %q: %w", subject, err)
	}
	return nil
}

// WorkspaceMemberRemove removes a workspace member through OpenShell RPC.
func (c *Client) WorkspaceMemberRemove(ctx context.Context, name, subject string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	if err := client.Workspaces().RemoveMember(ctx, name, subject); err != nil {
		return fmt.Errorf("remove OpenShell workspace member %q: %w", subject, err)
	}
	return nil
}

func providerRecordFromOpenShell(provider *openshell.Provider) gc.ProviderRecord {
	record := gc.ProviderRecord{
		Name:                  provider.Name,
		Type:                  provider.Type,
		Workspace:             provider.Workspace,
		Config:                gc.ProviderConfig(provider.Spec.Config),
		Credentials:           gc.Credentials{},
		CredentialExpiresAtMS: gc.CredentialExpiry{},
	}
	record.RuntimeCredentials, _ = strconv.ParseBool(provider.Annotations[runtimeCredentialsAnnotation])
	for name, expires := range provider.Spec.CredentialExpiresAt {
		record.CredentialExpiresAtMS[name] = expires.UnixMilli()
	}
	for name := range provider.Spec.Credentials {
		record.EnvVars = append(record.EnvVars, name)
	}
	sort.Strings(record.EnvVars)
	return record
}

func openShellRefreshStrategy(strategy string) (openshell.RefreshStrategy, bool) {
	switch strings.ToLower(strings.ReplaceAll(strategy, "_", "-")) {
	case "oauth2-refresh-token":
		return openshell.RefreshStrategyOAuth2RefreshToken, true
	case "oauth2-client-credentials":
		return openshell.RefreshStrategyOAuth2ClientCredentials, true
	default:
		return "", false
	}
}

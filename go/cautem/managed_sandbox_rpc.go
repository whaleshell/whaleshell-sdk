package cautem

import (
	"context"
	"fmt"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
)

// SyncManagedSandbox registers or updates metadata for a CLI-owned runtime.
// The RPC never provisions the runtime itself.
func (c *Client) SyncManagedSandbox(ctx context.Context, sandbox Sandbox) error {
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	workspace := sandbox.Workspace
	if workspace == "" {
		workspace = c.workspace()
	}
	_, err = controlv1.NewManagedSandboxServiceClient(conn).SyncManagedSandbox(ctx, &controlv1.SyncManagedSandboxRequest{
		Workspace: workspace, Name: sandbox.Name, RuntimeId: sandbox.ID, Image: sandbox.Image,
		Network: sandbox.Network, Status: sandbox.Status, Labels: sandbox.Labels,
		BasePolicyYaml: sandbox.BasePolicyYAML, AttachedProviders: sandbox.AttachedProviders,
	})
	if err != nil {
		return fmt.Errorf("sync managed sandbox: %w", err)
	}
	return nil
}

// GetManagedSandbox returns workspace scoped metadata for a CLI-owned runtime.
func (c *Client) GetManagedSandbox(ctx context.Context, name string) (Sandbox, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Sandbox{}, err
	}
	response, err := controlv1.NewManagedSandboxServiceClient(conn).GetManagedSandbox(ctx, &controlv1.GetManagedSandboxRequest{Workspace: c.workspace(), Name: name})
	if err != nil {
		return Sandbox{}, fmt.Errorf("get managed sandbox: %w", err)
	}
	item := response
	return Sandbox{Name: item.GetName(), ID: item.GetRuntimeId(), Image: item.GetImage(), Workspace: item.GetWorkspace(), Network: item.GetNetwork(), Status: item.GetStatus(), Labels: item.GetLabels(), BasePolicyYAML: item.GetBasePolicyYaml(), AttachedProviders: item.GetAttachedProviders(), ResourceVersion: item.GetResourceVersion()}, nil
}

// DeleteManagedSandbox removes the cautem registry entry after the CLI has
// already stopped and deleted the external runtime.
func (c *Client) DeleteManagedSandbox(ctx context.Context, name string) error {
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	_, err = controlv1.NewManagedSandboxServiceClient(conn).DeleteManagedSandbox(ctx, &controlv1.DeleteManagedSandboxRequest{Workspace: c.workspace(), Name: name})
	if err != nil {
		return fmt.Errorf("delete managed sandbox registry entry: %w", err)
	}
	return nil
}

// IssueManagedSandboxToken rotates and returns the supervisor token once.
func (c *Client) IssueManagedSandboxToken(ctx context.Context, name string) (string, error) {
	conn, err := c.controlConn()
	if err != nil {
		return "", err
	}
	response, err := controlv1.NewManagedSandboxServiceClient(conn).IssueManagedSandboxToken(ctx, &controlv1.IssueManagedSandboxTokenRequest{Workspace: c.workspace(), Name: name})
	if err != nil {
		return "", fmt.Errorf("issue managed sandbox token: %w", err)
	}
	return response.GetToken(), nil
}

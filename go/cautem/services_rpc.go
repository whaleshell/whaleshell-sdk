package cautem

import (
	"context"
	"fmt"

	openshell "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	gc "github.com/cautem/cautem-sdk/internal/gatewayclient"
)

// ExposeService registers a sandbox service through the pinned OpenShell RPC.
func (c *Client) ExposeService(ctx context.Context, sandbox, name string, port uint32) (gc.ServiceRecord, error) {
	client, err := c.openShellClient()
	if err != nil {
		return gc.ServiceRecord{}, err
	}
	endpoint, err := client.Services().Expose(ctx, c.workspace(), sandbox, name, port, true)
	if err != nil {
		return gc.ServiceRecord{}, fmt.Errorf("expose OpenShell service %q: %w", name, err)
	}
	return serviceRecord(endpoint), nil
}

// GetService reads a service endpoint through the pinned OpenShell RPC.
func (c *Client) GetService(ctx context.Context, name string) (gc.ServiceRecord, error) {
	client, err := c.openShellClient()
	if err != nil {
		return gc.ServiceRecord{}, err
	}
	endpoint, err := client.Services().Get(ctx, c.workspace(), "", name)
	if err != nil {
		return gc.ServiceRecord{}, fmt.Errorf("get OpenShell service %q: %w", name, err)
	}
	return serviceRecord(endpoint), nil
}

// PutService exposes a service over OpenShell RPC. Custom host routing is not
// accepted because OpenShell routes the endpoint through the sandbox relay.
func (c *Client) PutService(ctx context.Context, record gc.ServiceRecord) (gc.ServiceRecord, error) {
	if record.BackendHost != "" || (record.BackendPort != 0 && record.BackendPort != record.Port) {
		return gc.ServiceRecord{}, fmt.Errorf("custom service backends are not supported; use a sandbox target port")
	}
	if record.Port <= 0 || record.Port > 65535 {
		return gc.ServiceRecord{}, fmt.Errorf("service target port must be between 1 and 65535")
	}
	return c.ExposeService(ctx, record.Sandbox, record.Name, uint32(record.Port))
}

// DeleteService removes a service endpoint through the pinned OpenShell RPC.
func (c *Client) DeleteService(ctx context.Context, name string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	if err := client.Services().Delete(ctx, c.workspace(), "", name); err != nil {
		return fmt.Errorf("delete OpenShell service %q: %w", name, err)
	}
	return nil
}

func serviceRecord(endpoint *openshell.ServiceEndpoint) gc.ServiceRecord {
	if endpoint == nil {
		return gc.ServiceRecord{}
	}
	return gc.ServiceRecord{
		Name: endpoint.ServiceName, Sandbox: endpoint.SandboxName,
		Port: int(endpoint.TargetPort), BackendPort: int(endpoint.TargetPort),
	}
}

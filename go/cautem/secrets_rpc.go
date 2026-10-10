package cautem

import (
	"context"
	"fmt"

	openshellv1 "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
)

// ResolveSecrets resolves environment credentials for the matching sandbox
// principal. The gateway validates the sandbox token and returns only the
// environment the sandbox is allowed to receive.
func (c *Client) ResolveSecrets(ctx context.Context, sandbox string) (map[string]string, error) {
	if sandbox == "" {
		return nil, fmt.Errorf("sandbox name required")
	}
	client, err := c.openShellRPC()
	if err != nil {
		return nil, err
	}
	response, err := client.GetSandboxProviderEnvironment(ctx, &openshellv1.GetSandboxProviderEnvironmentRequest{SandboxId: sandbox})
	if err != nil {
		return nil, fmt.Errorf("resolve sandbox credentials: %w", err)
	}
	return response.GetEnvironment(), nil
}

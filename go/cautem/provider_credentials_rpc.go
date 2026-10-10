package cautem

import (
	"context"
	"fmt"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
)

// UpdateProviderCredentials patches credential values without reading or
// replacing existing secrets.
func (c *Client) UpdateProviderCredentials(ctx context.Context, workspace, name string, credentials map[string]string) error {
	if workspace == "" {
		workspace = c.workspace()
	}
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	_, err = controlv1.NewProviderCredentialServiceClient(conn).UpdateProviderCredentials(ctx, &controlv1.UpdateProviderCredentialsRequest{
		Workspace: workspace, ProviderName: name, Credentials: credentials,
	})
	if err != nil {
		return fmt.Errorf("update provider credentials for %q: %w", name, err)
	}
	return nil
}

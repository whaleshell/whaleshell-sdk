package cautem

import (
	"context"
	"fmt"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
)

func (c *Client) gatewayInfo(ctx context.Context) (map[string]any, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewGatewayAdminServiceClient(conn).GetGatewayInfo(ctx, &controlv1.GetGatewayInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf("get gateway info: %w", err)
	}
	drivers := make([]any, 0, len(response.GetComputeDrivers()))
	for _, driver := range response.GetComputeDrivers() {
		drivers = append(drivers, map[string]any{"name": driver.GetName(), "state": driver.GetState()})
	}
	return map[string]any{
		"gateway_id": response.GetGatewayId(), "sandbox_count": response.GetSandboxCount(), "auth_mode": response.GetAuthMode(),
		"compute_drivers": drivers, "credential_drivers": response.GetCredentialDrivers(),
		"default_credential_driver": response.GetDefaultCredentialDriver(), "allow_unauthenticated": response.GetAllowUnauthenticated(),
		"oidc_issuer": response.GetOidcIssuer(), "ssh_session_ttl_s": response.GetSshSessionTtlSeconds(),
		"secrets_kek": map[string]any{
			"source": response.GetSecretsKekSource(), "pinned": response.GetSecretsKekPinned(),
			"warning": response.GetSecretsKekWarning(), "format": response.GetSecretsKekFormat(),
			"migration_needed": response.GetSecretsKekMigrationNeeded(),
		},
	}, nil
}

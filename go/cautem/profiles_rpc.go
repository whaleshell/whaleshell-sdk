package cautem

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
	gc "github.com/cautem/cautem-sdk/internal/gatewayclient"
)

// ListProfiles and its scoped form use the curated cautem control API.
func (c *Client) ListProfiles(ctx context.Context) ([]gc.ProfileInfo, error) {
	return c.ListProfilesScoped(ctx, "global", "")
}

func (c *Client) ListProfilesScoped(ctx context.Context, scope, workspace string) ([]gc.ProfileInfo, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewProviderProfileServiceClient(conn).ListProviderProfiles(ctx, &controlv1.ListProviderProfilesRequest{
		Workspace: profileWorkspace(scope, workspace),
	})
	if err != nil {
		return nil, fmt.Errorf("list provider profiles: %w", err)
	}
	result := make([]gc.ProfileInfo, 0, len(response.GetProfiles()))
	for _, profile := range response.GetProfiles() {
		if profile == nil {
			continue
		}
		result = append(result, gc.ProfileInfo{
			ID: profile.GetId(), Category: profile.GetCategory(),
			Source: profile.GetSource(), Scope: profile.GetScope(), ResourceVersion: profile.GetResourceVersion(),
		})
	}
	return result, nil
}

func (c *Client) GetProfile(ctx context.Context, id string) ([]byte, string, string, error) {
	return c.GetProfileScoped(ctx, id, "global", "")
}

func (c *Client) GetProfileScoped(ctx context.Context, id, scope, workspace string) ([]byte, string, string, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, "", "", err
	}
	response, err := controlv1.NewProviderProfileServiceClient(conn).GetProviderProfile(ctx, &controlv1.GetProviderProfileRequest{
		Workspace: profileWorkspace(scope, workspace), Id: id,
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("get provider profile %q: %w", id, err)
	}
	return []byte(response.GetProfileYaml()), response.GetSource(), strconv.FormatUint(response.GetResourceVersion(), 10), nil
}

func (c *Client) CreateProfile(ctx context.Context, id string, profile []byte) error {
	return c.CreateProfileScoped(ctx, id, profile, "global", "")
}

func (c *Client) CreateProfileScoped(ctx context.Context, id string, profile []byte, scope, workspace string) error {
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	if len(profile) == 0 || len(profile) > 1<<20 {
		return fmt.Errorf("profile document must be between 1 byte and 1 MiB")
	}
	_, err = controlv1.NewProviderProfileServiceClient(conn).ImportProviderProfile(ctx, &controlv1.ImportProviderProfileRequest{
		Workspace: profileWorkspace(scope, workspace), ProfileYaml: string(profile), Id: id,
	})
	if err != nil {
		return fmt.Errorf("import provider profile %q: %w", id, err)
	}
	return nil
}

func (c *Client) PutProfile(ctx context.Context, id string, profile []byte, expectedVersion string) error {
	return c.PutProfileScoped(ctx, id, profile, expectedVersion, "global", "")
}

func (c *Client) PutProfileScoped(ctx context.Context, id string, profile []byte, expectedVersion, scope, workspace string) error {
	version, err := strconv.ParseUint(strings.Trim(expectedVersion, `"`), 10, 64)
	if err != nil || version == 0 {
		return fmt.Errorf("profile update requires a positive resource version")
	}
	if len(profile) == 0 || len(profile) > 1<<20 {
		return fmt.Errorf("profile document must be between 1 byte and 1 MiB")
	}
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	_, err = controlv1.NewProviderProfileServiceClient(conn).UpdateProviderProfile(ctx, &controlv1.UpdateProviderProfileRequest{
		Workspace: profileWorkspace(scope, workspace), Id: id,
		ExpectedResourceVersion: version, ProfileYaml: string(profile),
	})
	if err != nil {
		return fmt.Errorf("update provider profile %q: %w", id, err)
	}
	return nil
}

func (c *Client) DeleteProfile(ctx context.Context, id string) error {
	return c.DeleteProfileScoped(ctx, id, "global", "")
}

func (c *Client) DeleteProfileScoped(ctx context.Context, id, scope, workspace string) error {
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	_, err = controlv1.NewProviderProfileServiceClient(conn).DeleteProviderProfile(ctx, &controlv1.DeleteProviderProfileRequest{
		Workspace: profileWorkspace(scope, workspace), Id: id,
	})
	if err != nil {
		return fmt.Errorf("delete provider profile %q: %w", id, err)
	}
	return nil
}

func profileWorkspace(scope, workspace string) string {
	if scope == "global" {
		return ""
	}
	return workspace
}

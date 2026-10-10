package cautem

import (
	"context"
	"fmt"
	"sort"
	"time"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
	gc "github.com/cautem/cautem-sdk/internal/gatewayclient"
)

// TemplateRecord is the caller-visible catalog view of a sandbox template.
type TemplateRecord struct {
	Name            string
	Workspace       string
	Image           string
	Providers       []string
	ResourceVersion uint64
	CreatedAt       time.Time
}

// ListServices lists services visible in the selected workspace through RPC.
// Backend addresses are intentionally omitted from the public response.
func (c *Client) ListServices(ctx context.Context) ([]gc.ServiceRecord, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewCatalogServiceClient(conn).ListServices(ctx, &controlv1.ListServicesRequest{Workspace: c.workspace()})
	if err != nil {
		return nil, fmt.Errorf("list control services: %w", err)
	}
	result := make([]gc.ServiceRecord, 0, len(response.GetServices()))
	for _, item := range response.GetServices() {
		result = append(result, gc.ServiceRecord{Name: item.GetName(), Sandbox: item.GetSandboxName(), Port: int(item.GetPort())})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// ListTemplates lists caller-visible templates in the selected workspace.
func (c *Client) ListTemplates(ctx context.Context) ([]TemplateRecord, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewCatalogServiceClient(conn).ListTemplates(ctx, &controlv1.ListTemplatesRequest{Workspace: c.workspace()})
	if err != nil {
		return nil, fmt.Errorf("list control templates: %w", err)
	}
	result := make([]TemplateRecord, 0, len(response.GetTemplates()))
	for _, item := range response.GetTemplates() {
		result = append(result, TemplateRecord{
			Name: item.GetName(), Workspace: item.GetWorkspace(), Image: item.GetImage(),
			Providers: append([]string(nil), item.GetProviders()...), ResourceVersion: item.GetResourceVersion(),
			CreatedAt: time.UnixMilli(item.GetCreatedAtUnixMs()),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

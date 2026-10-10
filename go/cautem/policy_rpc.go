package cautem

import (
	"context"
	"fmt"
	"strings"
	"time"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
)

func (c *Client) GetGlobalPolicy(ctx context.Context) ([]byte, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).GetGlobalPolicy(ctx, &controlv1.GetGlobalPolicyRequest{})
	if err != nil {
		return nil, fmt.Errorf("get global policy: %w", err)
	}
	return []byte(response.GetPolicyYaml()), nil
}

func (c *Client) PutGlobalPolicy(ctx context.Context, document []byte) error {
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	service := controlv1.NewPolicyServiceClient(conn)
	current, err := service.GetGlobalPolicy(ctx, &controlv1.GetGlobalPolicyRequest{})
	if err != nil {
		return fmt.Errorf("read global policy before update: %w", err)
	}
	_, err = service.UpdateGlobalPolicy(ctx, &controlv1.UpdateGlobalPolicyRequest{
		PolicyYaml: string(document), ExpectedResourceVersion: current.GetResourceVersion(), Clear: len(document) == 0,
	})
	if err != nil {
		return fmt.Errorf("update global policy: %w", err)
	}
	return nil
}

func (c *Client) GetSandboxPolicy(ctx context.Context, sandbox, view string) ([]byte, error) {
	view = strings.ToLower(strings.TrimSpace(view))
	if view == "" || view == "effective" {
		view = "full"
	}
	if view != "base" && view != "full" {
		return nil, fmt.Errorf("policy view must be base or full")
	}
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).GetSandboxPolicy(ctx, &controlv1.GetSandboxPolicyRequest{
		Workspace: c.workspace(), SandboxName: sandbox, View: view,
	})
	if err != nil {
		return nil, fmt.Errorf("get sandbox policy %q: %w", sandbox, err)
	}
	return []byte(response.GetPolicyYaml()), nil
}

func (c *Client) EffectivePolicy(ctx context.Context, sandbox string) ([]byte, error) {
	return c.GetSandboxPolicy(ctx, sandbox, "full")
}

func (c *Client) PutSandboxPolicy(ctx context.Context, sandbox string, document []byte) ([]byte, int, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, 0, err
	}
	service := controlv1.NewPolicyServiceClient(conn)
	revisions, err := service.ListSandboxPolicyRevisions(ctx, &controlv1.ListSandboxPolicyRevisionsRequest{
		Workspace: c.workspace(), SandboxName: sandbox,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("read sandbox policy revision before update: %w", err)
	}
	var expected uint64
	for _, revision := range revisions.GetRevisions() {
		if revision.GetRevision() > expected {
			expected = revision.GetRevision()
		}
	}
	response, err := service.UpdateSandboxPolicy(ctx, &controlv1.UpdateSandboxPolicyRequest{
		Workspace: c.workspace(), SandboxName: sandbox, BasePolicyYaml: string(document), ExpectedPolicyRevision: expected,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("update sandbox policy %q: %w", sandbox, err)
	}
	return []byte(response.GetEffectivePolicyYaml()), int(response.GetStrippedProviderRules()), nil
}

func (c *Client) ListPolicyRevisions(ctx context.Context, sandbox string) ([]PolicyRevisionMeta, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).ListSandboxPolicyRevisions(ctx, &controlv1.ListSandboxPolicyRevisionsRequest{
		Workspace: c.workspace(), SandboxName: sandbox,
	})
	if err != nil {
		return nil, fmt.Errorf("list sandbox policy revisions %q: %w", sandbox, err)
	}
	result := make([]PolicyRevisionMeta, 0, len(response.GetRevisions()))
	for _, revision := range response.GetRevisions() {
		if revision == nil {
			continue
		}
		result = append(result, PolicyRevisionMeta{
			Rev: int(revision.GetRevision()), UpdatedAt: time.UnixMilli(revision.GetUpdatedAtUnixMs()),
			Bytes: int(revision.GetBytes()), Status: revision.GetStatus(),
		})
	}
	return result, nil
}

func (c *Client) GetPolicyRevision(ctx context.Context, sandbox string, revision int) (string, error) {
	if revision <= 0 {
		return "", fmt.Errorf("policy revision must be positive")
	}
	conn, err := c.controlConn()
	if err != nil {
		return "", err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).GetSandboxPolicyRevision(ctx, &controlv1.GetSandboxPolicyRevisionRequest{
		Workspace: c.workspace(), SandboxName: sandbox, Revision: uint64(revision),
	})
	if err != nil {
		return "", fmt.Errorf("get sandbox policy revision %q/%d: %w", sandbox, revision, err)
	}
	return response.GetPolicyYaml(), nil
}

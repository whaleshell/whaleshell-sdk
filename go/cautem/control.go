package cautem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
)

// ControlOverview returns caller-visible gateway summary data through the
// curated control API.
func (c *Client) ControlOverview(ctx context.Context) (map[string]any, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	console := controlv1.NewConsoleServiceClient(conn)
	viewer, err := console.GetViewer(ctx, &controlv1.GetViewerRequest{})
	if err != nil {
		return nil, fmt.Errorf("get control viewer: %w", err)
	}
	workspace := c.workspace()
	overview, err := console.GetOverview(ctx, &controlv1.GetOverviewRequest{Workspace: workspace})
	if err != nil {
		return nil, fmt.Errorf("get control overview: %w", err)
	}
	capabilities, err := console.GetConsoleCapabilities(ctx, &controlv1.GetConsoleCapabilitiesRequest{})
	if err != nil {
		return nil, fmt.Errorf("get control capabilities: %w", err)
	}
	return map[string]any{
		"gateway_id":                  overview.GetGatewayId(),
		"workspace":                   workspace,
		"sandbox_count":               overview.GetSandboxCount(),
		"registry_running_count":      overview.GetRegistryRunningCount(),
		"snapshot_at_unix_ms":         overview.GetSnapshotAtUnixMs(),
		"auth_mode":                   viewer.GetIdentityProvider(),
		"roles":                       append([]string(nil), viewer.GetRoles()...),
		"scopes":                      append([]string(nil), viewer.GetScopes()...),
		"compute_drivers":             append([]string(nil), capabilities.GetComputeDrivers()...),
		"sandbox_lifecycle_available": capabilities.GetSandboxLifecycleAvailable(),
	}, nil
}

// Whoami returns the authenticated viewer from the curated control API.
func (c *Client) Whoami(ctx context.Context) (map[string]any, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	response, err := controlv1.NewConsoleServiceClient(conn).GetViewer(ctx, &controlv1.GetViewerRequest{})
	if err != nil {
		return nil, fmt.Errorf("get control viewer: %w", err)
	}
	return map[string]any{
		"auth":              "authenticated",
		"subject":           response.GetSubject(),
		"roles":             append([]string(nil), response.GetRoles()...),
		"scopes":            append([]string(nil), response.GetScopes()...),
		"identity_provider": response.GetIdentityProvider(),
	}, nil
}

// ListProposals returns visible policy proposals through the Control API.
func (c *Client) ListProposals(ctx context.Context, sandbox, status string) ([]Proposal, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	client := controlv1.NewPolicyServiceClient(conn)
	after := ""
	var result []Proposal
	for {
		response, callErr := client.ListPolicyProposals(ctx, &controlv1.ListPolicyProposalsRequest{
			Workspace: c.workspace(), SandboxName: sandbox, Status: status, PageSize: 100, AfterId: after,
		})
		if callErr != nil {
			return nil, fmt.Errorf("list control proposals: %w", callErr)
		}
		for _, item := range response.GetProposals() {
			result = append(result, proposalFromSummary(item))
		}
		after = response.GetNextAfterId()
		if after == "" {
			return result, nil
		}
	}
}

// GetProposal returns one visible policy proposal through the Control API.
func (c *Client) GetProposal(ctx context.Context, sandbox, id string) (Proposal, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Proposal{}, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).GetPolicyProposal(ctx, &controlv1.GetPolicyProposalRequest{
		Workspace: c.workspace(), Id: id,
	})
	if err != nil {
		return Proposal{}, fmt.Errorf("get control proposal %q: %w", id, err)
	}
	proposal := proposalFromSummary(response.GetProposal())
	if sandbox != "" && proposal.Sandbox != sandbox {
		return Proposal{}, fmt.Errorf("proposal %q not found in sandbox %q", id, sandbox)
	}
	return proposal, nil
}

// ApproveProposal approves a visible policy proposal through the Control API.
func (c *Client) ApproveProposal(ctx context.Context, sandbox, id string) (Proposal, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Proposal{}, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).ApprovePolicyProposal(ctx, &controlv1.ApprovePolicyProposalRequest{
		Workspace: c.workspace(), Id: id,
	})
	if err != nil {
		return Proposal{}, fmt.Errorf("approve control proposal %q: %w", id, err)
	}
	proposal := proposalFromSummary(response.GetProposal())
	if sandbox != "" && proposal.Sandbox != sandbox {
		return Proposal{}, fmt.Errorf("proposal %q not found in sandbox %q", id, sandbox)
	}
	return proposal, nil
}

// RejectProposal rejects a visible policy proposal through the Control API.
func (c *Client) RejectProposal(ctx context.Context, sandbox, id, reason string) (Proposal, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Proposal{}, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).RejectPolicyProposal(ctx, &controlv1.RejectPolicyProposalRequest{
		Workspace: c.workspace(), Id: id, Reason: reason,
	})
	if err != nil {
		return Proposal{}, fmt.Errorf("reject control proposal %q: %w", id, err)
	}
	proposal := proposalFromSummary(response.GetProposal())
	if sandbox != "" && proposal.Sandbox != sandbox {
		return Proposal{}, fmt.Errorf("proposal %q not found in sandbox %q", id, sandbox)
	}
	return proposal, nil
}

func proposalFromSummary(proposal *controlv1.PolicyProposalSummary) Proposal {
	if proposal == nil {
		return Proposal{}
	}
	return Proposal{
		ID: proposal.GetId(), Sandbox: proposal.GetSandboxName(), Status: proposal.GetStatus(),
		IntentSummary: proposal.GetIntentSummary(), RuleName: proposal.GetRuleName(),
		RuleYAML: proposal.GetRuleYaml(), Hosts: append([]string(nil), proposal.GetHosts()...),
		RejectionReason: proposal.GetRejectionReason(), ValidationResult: proposal.GetValidationResult(),
		SecurityFlagged: proposal.GetSecurityFlagged(),
		CreatedAt:       time.UnixMilli(proposal.GetCreatedAtUnixMs()),
		DecidedAt:       time.UnixMilli(proposal.GetDecidedAtUnixMs()),
	}
}

const maxControlLogWatches = 24

// ControlMutationResult identifies a lifecycle request so callers can recover
// its authoritative outcome after a timeout or broken connection.
type ControlMutationResult struct {
	Sandbox     Sandbox
	RequestID   string
	OperationID string
	Deleted     bool
}

// ControlOperation is the durable gateway record for one mutating request.
type ControlOperation struct {
	ID                    string
	RequestID             string
	Workspace             string
	Sandbox               string
	Action                string
	State                 string
	ErrorCode             string
	ResultRegistryStatus  string
	ResultResourceVersion uint64
}

// CreateControlSandbox creates a runtime sandbox through the authorized control RPC.
func (c *Client) CreateControlSandbox(ctx context.Context, sandbox Sandbox, command []string) (Sandbox, error) {
	result, err := c.CreateControlSandboxOperation(ctx, sandbox, command)
	return result.Sandbox, err
}

// CreateControlSandboxOperation creates a sandbox and returns durable request
// identifiers even when the RPC outcome is uncertain.
func (c *Client) CreateControlSandboxOperation(ctx context.Context, sandbox Sandbox, command []string) (ControlMutationResult, error) {
	conn, err := c.controlConn()
	if err != nil {
		return ControlMutationResult{}, err
	}
	client := controlv1.NewSandboxServiceClient(conn)
	requestID, err := newRequestID()
	if err != nil {
		return ControlMutationResult{}, err
	}
	result := ControlMutationResult{RequestID: requestID}
	workspace := sandbox.Workspace
	if workspace == "" {
		workspace = "default"
	}
	response, err := client.CreateSandbox(ctx, &controlv1.CreateSandboxRequest{
		Workspace: workspace, Name: sandbox.Name, Image: sandbox.Image,
		Labels: sandbox.Labels, Command: append([]string(nil), command...), RequestId: requestID,
	})
	if err != nil {
		return result, fmt.Errorf("create control sandbox %q (request %s): %w", sandbox.Name, requestID, err)
	}
	result.Sandbox = sandboxFromSummary(response.GetSandbox())
	result.OperationID = response.GetOperationId()
	return result, nil
}

// StartControlSandbox starts one caller-visible runtime sandbox.
func (c *Client) StartControlSandbox(ctx context.Context, workspace, name string) (Sandbox, error) {
	result, err := c.ChangeControlSandboxStateOperation(ctx, workspace, name, "start")
	return result.Sandbox, err
}

// StopControlSandbox stops one caller-visible runtime sandbox.
func (c *Client) StopControlSandbox(ctx context.Context, workspace, name string) (Sandbox, error) {
	result, err := c.ChangeControlSandboxStateOperation(ctx, workspace, name, "stop")
	return result.Sandbox, err
}

// DeleteControlSandbox deletes one caller-visible runtime sandbox.
func (c *Client) DeleteControlSandbox(ctx context.Context, workspace, name string) error {
	_, err := c.DeleteControlSandboxOperation(ctx, workspace, name)
	return err
}

// DeleteControlSandboxOperation deletes a sandbox and exposes its operation ID.
func (c *Client) DeleteControlSandboxOperation(ctx context.Context, workspace, name string) (ControlMutationResult, error) {
	current, err := c.GetControlSandbox(ctx, workspace, name)
	if err != nil {
		return ControlMutationResult{}, err
	}
	conn, err := c.controlConn()
	if err != nil {
		return ControlMutationResult{}, err
	}
	requestID, err := newRequestID()
	if err != nil {
		return ControlMutationResult{}, err
	}
	result := ControlMutationResult{RequestID: requestID}
	response, err := controlv1.NewSandboxServiceClient(conn).DeleteSandbox(ctx, &controlv1.DeleteSandboxRequest{
		Workspace: current.Workspace, Name: current.Name,
		ExpectedResourceVersion: current.ResourceVersion, RequestId: requestID,
	})
	if err != nil {
		return result, fmt.Errorf("delete control sandbox %q (request %s): %w", name, requestID, err)
	}
	result.Deleted = response.GetDeleted()
	result.OperationID = response.GetOperationId()
	if !response.GetDeleted() {
		return result, fmt.Errorf("delete control sandbox %q: gateway did not confirm deletion", name)
	}
	return result, nil
}

// ChangeControlSandboxStateOperation starts or stops a sandbox and exposes the
// request and operation IDs used for conflict and timeout recovery.
func (c *Client) ChangeControlSandboxStateOperation(ctx context.Context, workspace, name, action string) (ControlMutationResult, error) {
	current, err := c.GetControlSandbox(ctx, workspace, name)
	if err != nil {
		return ControlMutationResult{}, err
	}
	conn, err := c.controlConn()
	if err != nil {
		return ControlMutationResult{}, err
	}
	requestID, err := newRequestID()
	if err != nil {
		return ControlMutationResult{}, err
	}
	result := ControlMutationResult{RequestID: requestID}
	request := &controlv1.StartSandboxRequest{Workspace: current.Workspace, Name: current.Name, ExpectedResourceVersion: current.ResourceVersion, RequestId: requestID}
	client := controlv1.NewSandboxServiceClient(conn)
	switch action {
	case "start":
		response, callErr := client.StartSandbox(ctx, request)
		if callErr != nil {
			return result, fmt.Errorf("start control sandbox %q (request %s): %w", name, requestID, callErr)
		}
		result.Sandbox = sandboxFromSummary(response.GetSandbox())
		result.OperationID = response.GetOperationId()
		return result, nil
	case "stop":
		response, callErr := client.StopSandbox(ctx, &controlv1.StopSandboxRequest{
			Workspace: current.Workspace, Name: current.Name,
			ExpectedResourceVersion: current.ResourceVersion, RequestId: requestID,
		})
		if callErr != nil {
			return result, fmt.Errorf("stop control sandbox %q (request %s): %w", name, requestID, callErr)
		}
		result.Sandbox = sandboxFromSummary(response.GetSandbox())
		result.OperationID = response.GetOperationId()
		return result, nil
	default:
		return result, fmt.Errorf("unsupported sandbox action %q", action)
	}
}

// GetControlOperation resolves a lifecycle request by its idempotency key.
func (c *Client) GetControlOperation(ctx context.Context, workspace, requestID string) (ControlOperation, error) {
	conn, err := c.controlConn()
	if err != nil {
		return ControlOperation{}, err
	}
	response, err := controlv1.NewOperationsServiceClient(conn).GetOperation(ctx, &controlv1.GetOperationRequest{Workspace: workspace, RequestId: requestID})
	if err != nil {
		return ControlOperation{}, fmt.Errorf("get control operation %q: %w", requestID, err)
	}
	operation := response.GetOperation()
	if operation == nil {
		return ControlOperation{}, fmt.Errorf("get control operation %q: empty response", requestID)
	}
	return ControlOperation{
		ID: operation.GetId(), RequestID: operation.GetRequestId(), Workspace: operation.GetWorkspace(), Sandbox: operation.GetSandbox(),
		Action: operation.GetAction(), State: operation.GetState(), ErrorCode: operation.GetErrorCode(),
		ResultRegistryStatus: operation.GetResultRegistryStatus(), ResultResourceVersion: operation.GetResultResourceVersion(),
	}, nil
}

func newRequestID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate sandbox request id: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}

// ListControlSandboxes returns the caller-visible sandbox inventory through
// the allowlisted control API. An empty workspace selects the default
// workspace; allWorkspaces uses the catalog to enumerate accessible spaces.
func (c *Client) ListControlSandboxes(ctx context.Context, workspace string, allWorkspaces bool) ([]Sandbox, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	sandboxClient := controlv1.NewSandboxServiceClient(conn)
	workspaces := []string{workspace}
	if allWorkspaces {
		catalog := controlv1.NewCatalogServiceClient(conn)
		response, err := catalog.ListWorkspaces(ctx, &controlv1.ListWorkspacesRequest{})
		if err != nil {
			return nil, fmt.Errorf("list control workspaces: %w", err)
		}
		workspaces = make([]string, 0, len(response.GetWorkspaces()))
		for _, item := range response.GetWorkspaces() {
			if item.GetName() != "" {
				workspaces = append(workspaces, item.GetName())
			}
		}
	} else if workspaces[0] == "" {
		workspaces[0] = "default"
	}

	var result []Sandbox
	for _, name := range workspaces {
		pageToken := ""
		for {
			response, err := sandboxClient.ListSandboxes(ctx, &controlv1.ListSandboxesRequest{
				Workspace: name, PageSize: 100, PageToken: pageToken,
			})
			if err != nil {
				return nil, fmt.Errorf("list control sandboxes in workspace %q: %w", name, err)
			}
			for _, item := range response.GetSandboxes() {
				result = append(result, sandboxFromSummary(item))
			}
			pageToken = response.GetNextPageToken()
			if pageToken == "" {
				break
			}
		}
	}
	return result, nil
}

// GetControlSandbox returns one redacted sandbox summary through the control API.
func (c *Client) GetControlSandbox(ctx context.Context, workspace, name string) (Sandbox, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Sandbox{}, err
	}
	client := controlv1.NewSandboxServiceClient(conn)
	response, err := client.GetSandbox(ctx, &controlv1.GetSandboxRequest{
		Workspace: workspace, Name: name,
	})
	if err != nil {
		return Sandbox{}, fmt.Errorf("get control sandbox %q: %w", name, err)
	}
	return sandboxFromSummary(response.GetSandbox()), nil
}

// GetControlSandboxLogs returns a bounded tail of gateway-buffered logs.
func (c *Client) GetControlSandboxLogs(ctx context.Context, workspace, name string, limit uint32) ([]LogLine, error) {
	return c.GetControlSandboxLogsFiltered(ctx, workspace, name, limit, "", "", "")
}

// GetControlSandboxLogsFiltered returns a bounded tail with optional time,
// source, and level filters. since accepts a Go duration or RFC3339 timestamp.
func (c *Client) GetControlSandboxLogsFiltered(ctx context.Context, workspace, name string, limit uint32, since, source, level string) ([]LogLine, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	client := controlv1.NewSandboxServiceClient(conn)
	response, err := client.GetSandboxLogs(ctx, &controlv1.GetSandboxLogsRequest{
		Workspace: workspace, Name: name, Limit: limit, SinceUnixMs: parseLogSince(since),
		Source: source, Level: level,
	})
	if err != nil {
		return nil, fmt.Errorf("get control sandbox logs %q: %w", name, err)
	}
	lines := make([]LogLine, 0, len(response.GetLines()))
	for _, line := range response.GetLines() {
		lines = append(lines, LogLine{
			TS: time.UnixMilli(line.GetTimestampUnixMs()), Source: line.GetSource(),
			Level: line.GetLevel(), Text: line.GetMessage(),
		})
	}
	return lines, nil
}

// WatchControlSandboxLogs follows the gateway's bounded log stream. The
// gateway sends a fresh bounded tail on a new watch and the caller should
// reconnect with a new watch after a stream error.
func (c *Client) WatchControlSandboxLogs(ctx context.Context, workspace, name string, initialLimit uint32, w io.Writer) error {
	return c.WatchControlSandboxLogsFiltered(ctx, workspace, name, initialLimit, "", "", "", w)
}

// WatchControlSandboxLogsFiltered follows a filtered workspace-scoped log stream.
func (c *Client) WatchControlSandboxLogsFiltered(ctx context.Context, workspace, name string, initialLimit uint32, since, source, level string, w io.Writer) error {
	return c.watchControlSandboxLogs(ctx, workspace, name, initialLimit, parseLogSince(since), source, level, "", w)
}

// WatchControlSandboxSet follows every supplied sandbox concurrently.
func (c *Client) WatchControlSandboxSet(ctx context.Context, sandboxes []Sandbox, initialLimit uint32, since, source, level string, w io.Writer) error {
	if w == nil {
		return fmt.Errorf("watch control sandbox logs: writer required")
	}
	if len(sandboxes) == 0 {
		return nil
	}
	if len(sandboxes) > maxControlLogWatches {
		return fmt.Errorf("watch control sandbox logs: %d sandboxes exceed the concurrent watch limit of %d", len(sandboxes), maxControlLogWatches)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var writers sync.Mutex
	var wait sync.WaitGroup
	errCh := make(chan error, len(sandboxes))
	for _, sandbox := range sandboxes {
		sandbox := sandbox
		wait.Add(1)
		go func() {
			defer wait.Done()
			label := sandbox.Workspace + "/" + sandbox.Name
			err := c.watchControlSandboxLogs(watchCtx, sandbox.Workspace, sandbox.Name, initialLimit, parseLogSince(since), source, level, label, writerLocked{mu: &writers, w: w})
			if err != nil && watchCtx.Err() == nil {
				select {
				case errCh <- err:
				default:
				}
				cancel()
			}
		}()
	}
	wait.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

type writerLocked struct {
	mu *sync.Mutex
	w  io.Writer
}

func (w writerLocked) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func (c *Client) watchControlSandboxLogs(ctx context.Context, workspace, name string, initialLimit uint32, sinceUnixMS int64, source, level, label string, w io.Writer) error {
	if w == nil {
		return fmt.Errorf("watch control sandbox logs: writer required")
	}
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	client := controlv1.NewSandboxServiceClient(conn)
	stream, err := client.WatchSandboxLogs(ctx, &controlv1.WatchSandboxLogsRequest{
		Workspace: workspace, Name: name, InitialLimit: initialLimit, SinceUnixMs: sinceUnixMS,
		Source: source, Level: level,
	})
	if err != nil {
		return fmt.Errorf("watch control sandbox logs %q: %w", name, err)
	}
	for {
		event, recvErr := stream.Recv()
		if recvErr == io.EOF {
			return nil
		}
		if recvErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("watch control sandbox logs %q: %w", name, recvErr)
		}
		line := event.GetLine()
		if line == nil {
			continue
		}
		prefix := line.GetSource()
		if prefix == "" {
			prefix = "proxy"
		}
		if label != "" {
			prefix = label + " " + prefix
		}
		if _, err := fmt.Fprintf(w, "[%s] %s\n", prefix, line.GetMessage()); err != nil {
			return err
		}
	}
}

func parseLogSince(since string) int64 {
	if since == "" {
		return 0
	}
	if duration, err := time.ParseDuration(since); err == nil {
		return time.Now().UTC().Add(-duration).UnixMilli()
	}
	if timestamp, err := time.Parse(time.RFC3339Nano, since); err == nil {
		return timestamp.UnixMilli()
	}
	return 0
}

func sandboxFromSummary(item *controlv1.SandboxSummary) Sandbox {
	if item == nil {
		return Sandbox{}
	}
	return Sandbox{
		Name: item.GetName(), ID: item.GetId(), Image: item.GetImage(),
		Workspace: item.GetWorkspace(), ResourceVersion: item.GetResourceVersion(), Status: item.GetRegistryStatus(),
		Labels: item.GetLabels(),
	}
}

// Package gatewayclient talks to cautem-gateway HTTP API.
package gatewayclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RequestTimeout bounds ordinary gateway requests; RelayTimeout allows relay execution.
const (
	RequestTimeout = 10 * time.Second
	RelayTimeout   = 70 * time.Second
)

// Labels maps metadata names to their values.
type Labels = map[string]string

// Credentials maps credential environment keys to secret values.
type Credentials = map[string]string

// ProviderConfig maps provider option names to their values.
type ProviderConfig = map[string]string

// RefreshMaterial maps refresh input names to their values.
type RefreshMaterial = map[string]string

// CredentialBindings maps refresh inputs or outputs to credential keys.
type CredentialBindings = map[string]string

// CredentialExpiry maps credential keys to Unix millisecond expiration times.
type CredentialExpiry = map[string]int64

// Client is a tiny HTTP client for cautem-gateway.
type Client struct {
	Base  string
	Token string // optional Bearer
	HTTP  *http.Client
}

// New returns a client for base URL (for example http://127.0.0.1:7443).
// The gateway requires a bearer on every /v1 route; set Token (or use
// NewWithToken) — it is attached to every request by the transport.
func New(base string) *Client {
	c := &Client{Base: strings.TrimRight(base, "/")}
	c.HTTP = &http.Client{Timeout: RequestTimeout, Transport: &authTransport{c: c}}
	return c
}

// authTransport adds the bearer to requests that do not carry one, so no
// call site can forget authentication.
type authTransport struct {
	c    *Client
	base http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if tok := t.c.Token; tok != "" && req.Header.Get("Authorization") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return base.RoundTrip(req)
}

// NewWithToken returns a client with bearer auth.
func NewWithToken(base, token string) *Client {
	c := New(base)
	c.Token = strings.TrimSpace(token)
	return c
}

func (c *Client) auth(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

// Healthz hits /healthz.
func (c *Client) Healthz(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/healthz", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Sandbox is the registry payload.
type Sandbox struct {
	Name              string   `json:"name"`
	ID                string   `json:"id,omitempty"`
	Image             string   `json:"image,omitempty"`
	Workspace         string   `json:"workspace,omitempty"`
	ResourceVersion   uint64   `json:"resource_version,omitempty"`
	Network           string   `json:"network,omitempty"`
	Status            string   `json:"status,omitempty"`
	Labels            Labels   `json:"labels,omitempty"`
	BasePolicyYAML    string   `json:"base_policy_yaml,omitempty"`
	AttachedProviders []string `json:"attached_providers,omitempty"`
}

// UpsertSandbox PUT /v1/sandboxes/{name}.
func (c *Client) UpsertSandbox(ctx context.Context, sb Sandbox) error {
	b, err := json.Marshal(sb)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/sandboxes/"+url.PathEscape(sb.Name), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway upsert: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// DeleteSandbox DELETE /v1/sandboxes/{name}.
func (c *Client) DeleteSandbox(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/sandboxes/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode != http.StatusNotFound {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway delete: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ListSandboxes GET /v1/sandboxes.
func (c *Client) ListSandboxes(ctx context.Context) ([]Sandbox, error) {
	var out struct {
		Sandboxes []Sandbox `json:"sandboxes"`
	}
	if err := c.get(ctx, "/v1/sandboxes", &out); err != nil {
		return nil, err
	}
	return out.Sandboxes, nil
}

// ProfileInfo is a catalog entry summary.
type ProfileInfo struct {
	ID              string `json:"id"`
	Category        string `json:"category,omitempty"`
	Source          string `json:"source"`
	Scope           string `json:"scope,omitempty"`
	ResourceVersion uint64 `json:"resource_version,omitempty"`
}

// ProviderRecord is a gateway provider instance (env key names only on GET).
type ProviderRecord struct {
	Name                  string                           `json:"name"`
	Type                  string                           `json:"type"`
	Workspace             string                           `json:"workspace,omitempty"`
	EnvVars               []string                         `json:"env_vars,omitempty"`
	Credentials           Credentials                      `json:"credentials,omitempty"` // write-only on PUT
	CredentialExpiresAtMS CredentialExpiry                 `json:"credential_expires_at_ms,omitempty"`
	RuntimeCredentials    bool                             `json:"runtime_credentials,omitempty"`
	Config                ProviderConfig                   `json:"config,omitempty"`
	Refresh               map[string]ProviderRefreshConfig `json:"refresh,omitempty"`
}

// ProviderRefreshConfig is gateway-side credential rotation metadata.
type ProviderRefreshConfig struct {
	CredentialKey          string             `json:"credential_key"`
	Strategy               string             `json:"strategy"`
	Material               RefreshMaterial    `json:"material,omitempty"`
	MaterialSecretKeys     []string           `json:"material_secret_keys,omitempty"`
	MaterialCredentialKeys CredentialBindings `json:"material_credential_keys,omitempty"`
	Outputs                CredentialBindings `json:"outputs,omitempty"`
	RefreshBeforeSeconds   int64              `json:"refresh_before_seconds,omitempty"`
	MaxLifetimeSeconds     int64              `json:"max_lifetime_seconds,omitempty"`
	ExpiresAtMS            int64              `json:"expires_at_ms,omitempty"`
}

// PutProvider PUT /v1/providers/{name}. Credentials values are stored encrypted on the gateway.
func (c *Client) PutProvider(ctx context.Context, rec ProviderRecord) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/providers/"+url.PathEscape(rec.Name), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway put provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ListProviders GET /v1/providers.
func (c *Client) ListProviders(ctx context.Context) ([]ProviderRecord, error) {
	var out struct {
		Providers []ProviderRecord `json:"providers"`
	}
	if err := c.get(ctx, "/v1/providers", &out); err != nil {
		return nil, err
	}
	return out.Providers, nil
}

// GetProvider GET /v1/providers/{name} (metadata only; no secret values).
func (c *Client) GetProvider(ctx context.Context, name string) (ProviderRecord, error) {
	var out ProviderRecord
	if err := c.get(ctx, "/v1/providers/"+url.PathEscape(name), &out); err != nil {
		return ProviderRecord{}, err
	}
	return out, nil
}

// DeleteProvider DELETE /v1/providers/{name}.
func (c *Client) DeleteProvider(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/providers/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway delete provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// AttachProvider PUT /v1/sandboxes/{sandbox}/providers/{provider}.
func (c *Client) AttachProvider(ctx context.Context, sandbox, provider string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/providers/"+url.PathEscape(provider), nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway attach provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// DetachProvider DELETE /v1/sandboxes/{sandbox}/providers/{provider}.
func (c *Client) DetachProvider(ctx context.Context, sandbox, provider string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/providers/"+url.PathEscape(provider), nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway detach provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// PolicyRevisionMeta is metadata for policy list (no YAML body).
type PolicyRevisionMeta struct {
	Rev       int       `json:"rev"`
	UpdatedAt time.Time `json:"updated_at"`
	Bytes     int       `json:"bytes"`
	Status    string    `json:"status"`
}

// SandboxProviderAttachment is metadata for sandbox provider list.
type SandboxProviderAttachment struct {
	Name    string   `json:"name"`
	Type    string   `json:"type,omitempty"`
	EnvVars []string `json:"env_vars,omitempty"`
}

// ListSandboxProviders GET /v1/sandboxes/{name}/providers.
func (c *Client) ListSandboxProviders(ctx context.Context, sandbox string) ([]SandboxProviderAttachment, error) {
	var out struct {
		Providers []SandboxProviderAttachment `json:"providers"`
	}
	if err := c.get(ctx, "/v1/sandboxes/"+url.PathEscape(sandbox)+"/providers", &out); err != nil {
		return nil, err
	}
	return out.Providers, nil
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return err
	}
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway %s: %s: %s", path, res.Status, bytes.TrimSpace(body))
	}
	return json.NewDecoder(res.Body).Decode(dest)
}

// ResolveSecrets GET /v1/sandboxes/{name}/secrets — sidecar credential map.
func (c *Client) ResolveSecrets(ctx context.Context, sandbox string) (map[string]string, error) {
	var out struct {
		Secrets map[string]string `json:"secrets"`
	}
	if err := c.get(ctx, "/v1/sandboxes/"+url.PathEscape(sandbox)+"/secrets", &out); err != nil {
		return nil, err
	}
	if out.Secrets == nil {
		out.Secrets = map[string]string{}
	}
	return out.Secrets, nil
}

// PostLogs POST /v1/sandboxes/{name}/logs — ingest observation lines.
func (c *Client) PostLogs(ctx context.Context, sandbox string, lines []LogLine) error {
	b, err := json.Marshal(map[string]any{"lines": lines})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/logs", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway post logs: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// Proposal is a policy.local chunk stored on the gateway.
type Proposal struct {
	ID               string    `json:"id"`
	Sandbox          string    `json:"sandbox"`
	Status           string    `json:"status"`
	IntentSummary    string    `json:"intent_summary,omitempty"`
	RuleName         string    `json:"rule_name,omitempty"`
	RuleYAML         string    `json:"rule_yaml,omitempty"`
	Hosts            []string  `json:"hosts,omitempty"`
	RejectionReason  string    `json:"rejection_reason,omitempty"`
	ValidationResult string    `json:"validation_result,omitempty"`
	SecurityFlagged  bool      `json:"security_flagged,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	DecidedAt        time.Time `json:"decided_at"`
}

// ListProposals GET /v1/sandboxes/{name}/proposals.
func (c *Client) ListProposals(ctx context.Context, sandbox, status string) ([]Proposal, error) {
	path := "/v1/sandboxes/" + url.PathEscape(sandbox) + "/proposals"
	if status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	var out struct {
		Proposals []Proposal `json:"proposals"`
	}
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out.Proposals, nil
}

// GetProposal GET /v1/sandboxes/{name}/proposals/{id}.
func (c *Client) GetProposal(ctx context.Context, sandbox, id string) (Proposal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/proposals/"+url.PathEscape(id), nil)
	if err != nil {
		return Proposal{}, err
	}
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Proposal{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("gateway read response body: %w", err)
		return Proposal{}, err
	}
	if res.StatusCode >= 300 {
		return Proposal{}, fmt.Errorf("get proposal: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var p Proposal
	if err := json.Unmarshal(body, &p); err != nil {
		return Proposal{}, err
	}
	return p, nil
}

// ApproveProposal POST /v1/sandboxes/{name}/proposals/{id}/approve — merges rule into base.
func (c *Client) ApproveProposal(ctx context.Context, sandbox, id string) (Proposal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/proposals/"+url.PathEscape(id)+"/approve", nil)
	if err != nil {
		return Proposal{}, err
	}
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Proposal{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("gateway read response body: %w", err)
		return Proposal{}, err
	}
	if res.StatusCode >= 300 {
		return Proposal{}, fmt.Errorf("approve proposal: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var p Proposal
	if err := json.Unmarshal(body, &p); err != nil {
		return Proposal{}, err
	}
	return p, nil
}

// RejectProposal POST /v1/sandboxes/{name}/proposals/{id}/reject.
func (c *Client) RejectProposal(ctx context.Context, sandbox, id, reason string) (Proposal, error) {
	b, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		err = fmt.Errorf("gateway encode request: %w", err)
		return Proposal{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/proposals/"+url.PathEscape(id)+"/reject", bytes.NewReader(b))
	if err != nil {
		return Proposal{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Proposal{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("gateway read response body: %w", err)
		return Proposal{}, err
	}
	if res.StatusCode >= 300 {
		return Proposal{}, fmt.Errorf("reject proposal: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var p Proposal
	if err := json.Unmarshal(body, &p); err != nil {
		return Proposal{}, err
	}
	return p, nil
}

// LogLine is one observation event for ingest/SSE.
type LogLine struct {
	TS     time.Time `json:"ts"`
	Source string    `json:"source"`
	Level  string    `json:"level"`
	Text   string    `json:"text"`
}

// FollowLogs streams SSE log lines to w. names may be multiple; all=true uses gateway ?all=1.
func (c *Client) FollowLogs(ctx context.Context, names []string, all bool, since, source, level string, w io.Writer) error {
	path := "/v1/logs"
	q := url.Values{"follow": {"1"}}
	if all {
		q.Set("all", "1")
	}
	for _, name := range names {
		q.Add("name", name)
	}
	if since != "" {
		q.Set("since", since)
	}
	if source != "" {
		q.Set("source", source)
	}
	if level != "" {
		q.Set("level", level)
	}
	if !all && len(names) == 1 {
		path = "/v1/sandboxes/" + url.PathEscape(names[0]) + "/logs"
		q.Del("name")
	}
	u := c.Base + path + "?" + q.Encode()

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	// SSE needs no overall timeout
	sseClient := *httpClient
	sseClient.Timeout = 0

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	res, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("gateway read response body: %w", err)
			return err
		}
		return fmt.Errorf("gateway follow logs: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			if _, err := fmt.Fprintln(w, after); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// GetLogsSnapshot returns recent log lines (non-follow JSON).
func (c *Client) GetLogsSnapshot(ctx context.Context, name, since, source, level string) ([]LogLine, error) {
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	if source != "" {
		q.Set("source", source)
	}
	if level != "" {
		q.Set("level", level)
	}
	path := "/v1/sandboxes/" + url.PathEscape(name) + "/logs?" + q.Encode()
	var out struct {
		Lines []LogLine `json:"lines"`
	}
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out.Lines, nil
}

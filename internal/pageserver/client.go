// Package pageserver is a thin client for the Neon pageserver HTTP API.
//
// A Neon "branch" is a timeline. Branching = creating a child timeline that
// points at a parent timeline + LSN (copy-on-write at the storage layer).
package pageserver

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to a single pageserver HTTP endpoint (default :9898).
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a Client for the given base URL, e.g. "http://localhost:9898".
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Timeline is one branch.
type Timeline struct {
	TimelineID         string `json:"timeline_id"`
	AncestorTimelineID string `json:"ancestor_timeline_id,omitempty"`
	AncestorLSN        string `json:"ancestor_lsn,omitempty"`
	LastRecordLSN      string `json:"last_record_lsn,omitempty"`
}

// Tenant identifies a project's storage.
type Tenant struct {
	ID string `json:"id"`
}

// GenID returns a random 32-hex-char id suitable for tenant/timeline ids.
func GenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (c *Client) do(method, path string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pageserver request failed (is the stack up?): %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pageserver %s %s: %s: %s", method, path, resp.Status, string(data))
	}
	return data, nil
}

// Tenants lists all tenants.
func (c *Client) Tenants() ([]Tenant, error) {
	data, err := c.do(http.MethodGet, "/v1/tenant", nil)
	if err != nil {
		return nil, err
	}
	var out []Tenant
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DefaultTenant returns the first tenant, the common case in a single-project
// dev stack.
func (c *Client) DefaultTenant() (string, error) {
	ts, err := c.Tenants()
	if err != nil {
		return "", err
	}
	if len(ts) == 0 {
		return "", fmt.Errorf("no tenant found; start a compute first to bootstrap one")
	}
	return ts[0].ID, nil
}

// CreateTenant creates a tenant (location_config in AttachedSingle mode). The
// tenant starts in Attaching and must reach Active before timelines can be
// created; use WaitTenantActive.
func (c *Client) CreateTenant(tenant string) error {
	body := map[string]any{"mode": "AttachedSingle", "generation": 1, "tenant_conf": map[string]any{}}
	_, err := c.do(http.MethodPut, "/v1/tenant/"+tenant+"/location_config", body)
	return err
}

// TenantState returns the tenant's lifecycle slug, e.g. "Attaching", "Active".
func (c *Client) TenantState(tenant string) (string, error) {
	data, err := c.do(http.MethodGet, "/v1/tenant/"+tenant, nil)
	if err != nil {
		return "", err
	}
	var out struct {
		State struct {
			Slug string `json:"slug"`
		} `json:"state"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	return out.State.Slug, nil
}

// WaitTenantActive polls until the tenant is Active or the deadline passes.
func (c *Client) WaitTenantActive(tenant string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		st, err := c.TenantState(tenant)
		if err == nil && st == "Active" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tenant %s not active within %s (last state: %q)", tenant, timeout, st)
		}
		time.Sleep(2 * time.Second)
	}
}

// Timelines lists the branches of a tenant.
func (c *Client) Timelines(tenant string) ([]Timeline, error) {
	data, err := c.do(http.MethodGet, "/v1/tenant/"+tenant+"/timeline", nil)
	if err != nil {
		return nil, err
	}
	var out []Timeline
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Timeline fetches a single timeline (used to read its current LSN).
func (c *Client) Timeline(tenant, timeline string) (*Timeline, error) {
	data, err := c.do(http.MethodGet, "/v1/tenant/"+tenant+"/timeline/"+timeline, nil)
	if err != nil {
		return nil, err
	}
	var out Timeline
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateBranch creates a child timeline off (ancestor, lsn). If ancestor is
// empty it creates a root timeline. If lsn is empty the branch point is the
// ancestor's current last_record_lsn.
func (c *Client) CreateBranch(tenant, newID, ancestor, lsn string, pgVersion int) (*Timeline, error) {
	body := map[string]any{
		"new_timeline_id": newID,
		"pg_version":      pgVersion,
	}
	if ancestor != "" {
		if lsn == "" {
			anc, err := c.Timeline(tenant, ancestor)
			if err != nil {
				return nil, err
			}
			lsn = anc.LastRecordLSN
		}
		body["ancestor_timeline_id"] = ancestor
		body["ancestor_start_lsn"] = lsn
	}
	data, err := c.do(http.MethodPost, "/v1/tenant/"+tenant+"/timeline/", body)
	if err != nil {
		return nil, err
	}
	var out Timeline
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteBranch deletes a timeline.
func (c *Client) DeleteBranch(tenant, timeline string) error {
	_, err := c.do(http.MethodDelete, "/v1/tenant/"+tenant+"/timeline/"+timeline, nil)
	return err
}

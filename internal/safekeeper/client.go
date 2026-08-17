// Package safekeeper is a thin client for the Neon safekeeper HTTP API.
//
// Unlike the pageserver, a timeline's WAL lives independently on every
// safekeeper in the quorum. Deleting a timeline from the pageserver does not
// remove it from any safekeeper — each one must be told to delete it
// separately, or its on-disk WAL segments become permanent orphans.
package safekeeper

import (
	"fmt"
	"net/http"
	"time"
)

// Client talks to one safekeeper's HTTP management API (default :7676).
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a Client for the given base URL, e.g. "http://host:7676".
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// DeleteTimeline deletes a timeline's WAL directory on this safekeeper.
// A safekeeper that never held the timeline (e.g. it was never part of the
// quorum, or the directory was already removed) answers 200 with
// dir_existed:false — treated as success, not an error.
func (c *Client) DeleteTimeline(tenant, timeline string) error {
	req, err := http.NewRequest(http.MethodDelete,
		c.BaseURL+"/v1/tenant/"+tenant+"/timeline/"+timeline, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("safekeeper %s request failed: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("safekeeper %s DELETE timeline/%s: %s", c.BaseURL, timeline, resp.Status)
	}
	return nil
}

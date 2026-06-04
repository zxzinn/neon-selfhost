// Package state persists branch metadata (name, timeline id, host port) that
// the pageserver itself does not track. Stored as JSON under ~/.neon-selfhost.
//
// Keyed by tenant so multiple projects don't collide.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Branch is one named branch the CLI manages.
type Branch struct {
	Name       string `json:"name"`
	TimelineID string `json:"timeline_id"`
	Ancestor   string `json:"ancestor"`
	Port       int    `json:"port"`
}

// Store is the on-disk state, keyed by tenant id.
type Store struct {
	path     string
	Tenants  map[string]*tenantState `json:"tenants"`
}

type tenantState struct {
	Branches []Branch `json:"branches"`
}

// dir returns the state directory, honoring NEON_SELFHOST_HOME for tests.
func dir() (string, error) {
	if d := os.Getenv("NEON_SELFHOST_HOME"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".neon-selfhost"), nil
}

// Load reads the store, returning an empty one if the file doesn't exist.
func Load() (*Store, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	p := filepath.Join(d, "state.json")
	s := &Store{path: p, Tenants: map[string]*tenantState{}}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Tenants == nil {
		s.Tenants = map[string]*tenantState{}
	}
	s.path = p
	return s, nil
}

// Save writes the store atomically.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) tenant(id string) *tenantState {
	if s.Tenants[id] == nil {
		s.Tenants[id] = &tenantState{}
	}
	return s.Tenants[id]
}

// List returns the branches for a tenant.
func (s *Store) List(tenant string) []Branch {
	return s.tenant(tenant).Branches
}

// Get finds a branch by name within a tenant.
func (s *Store) Get(tenant, name string) (*Branch, bool) {
	for i := range s.tenant(tenant).Branches {
		b := &s.tenant(tenant).Branches[i]
		if b.Name == name {
			return b, true
		}
	}
	return nil, false
}

// ByTimeline finds a branch by timeline id.
func (s *Store) ByTimeline(tenant, timeline string) (*Branch, bool) {
	for i := range s.tenant(tenant).Branches {
		b := &s.tenant(tenant).Branches[i]
		if b.TimelineID == timeline {
			return b, true
		}
	}
	return nil, false
}

// Add records a new branch. Errors if the name is taken.
func (s *Store) Add(tenant string, b Branch) error {
	if _, ok := s.Get(tenant, b.Name); ok {
		return fmt.Errorf("branch name %q already exists", b.Name)
	}
	t := s.tenant(tenant)
	t.Branches = append(t.Branches, b)
	return nil
}

// Remove deletes a branch by name.
func (s *Store) Remove(tenant, name string) bool {
	t := s.tenant(tenant)
	for i := range t.Branches {
		if t.Branches[i].Name == name {
			t.Branches = append(t.Branches[:i], t.Branches[i+1:]...)
			return true
		}
	}
	return false
}

// AllocPort returns the lowest free port in [base, base+span) not used by any
// recorded branch in any tenant.
func (s *Store) AllocPort(base, span int) (int, error) {
	used := map[int]bool{}
	for _, t := range s.Tenants {
		for _, b := range t.Branches {
			used[b.Port] = true
		}
	}
	for p := base; p < base+span; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port in range %d-%d", base, base+span-1)
}

// Names returns the sorted branch names for a tenant (for help/validation).
func (s *Store) Names(tenant string) []string {
	var out []string
	for _, b := range s.tenant(tenant).Branches {
		out = append(out, b.Name)
	}
	sort.Strings(out)
	return out
}

package pageserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenID(t *testing.T) {
	id, err := GenID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("GenID len = %d, want 32", len(id))
	}
	id2, _ := GenID()
	if id == id2 {
		t.Fatal("GenID returned identical ids")
	}
}

func TestDefaultTenant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"id":"tenant-1"},{"id":"tenant-2"}]`)
	}))
	defer srv.Close()
	got, err := New(srv.URL).DefaultTenant()
	if err != nil || got != "tenant-1" {
		t.Fatalf("DefaultTenant = %q, %v; want tenant-1", got, err)
	}
}

func TestDefaultTenantEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	if _, err := New(srv.URL).DefaultTenant(); err == nil {
		t.Fatal("expected error when no tenant exists")
	}
}

// TestCreateBranchExplicitLSN verifies the POST body when ancestor + lsn given.
func TestCreateBranchExplicitLSN(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, `{"timeline_id":"new","ancestor_timeline_id":"main","ancestor_lsn":"0/ABC"}`)
	}))
	defer srv.Close()

	tl, err := New(srv.URL).CreateBranch("t1", "new", "main", "0/ABC", 16)
	if err != nil {
		t.Fatal(err)
	}
	if tl.TimelineID != "new" {
		t.Fatalf("timeline = %q", tl.TimelineID)
	}
	if body["new_timeline_id"] != "new" || body["ancestor_timeline_id"] != "main" || body["ancestor_start_lsn"] != "0/ABC" {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body["pg_version"].(float64) != 16 {
		t.Fatalf("pg_version = %v", body["pg_version"])
	}
}

// TestCreateBranchDefaultsLSN verifies that with ancestor but no lsn, the client
// fetches the ancestor's last_record_lsn and uses it as the branch point.
func TestCreateBranchDefaultsLSN(t *testing.T) {
	var postBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tenant/t1/timeline/main":
			io.WriteString(w, `{"timeline_id":"main","last_record_lsn":"0/DEAD"}`)
		case r.Method == http.MethodPost:
			json.NewDecoder(r.Body).Decode(&postBody)
			io.WriteString(w, `{"timeline_id":"new","ancestor_timeline_id":"main","ancestor_lsn":"0/DEAD"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	if _, err := New(srv.URL).CreateBranch("t1", "new", "main", "", 16); err != nil {
		t.Fatal(err)
	}
	if postBody["ancestor_start_lsn"] != "0/DEAD" {
		t.Fatalf("expected defaulted lsn 0/DEAD, got %v", postBody["ancestor_start_lsn"])
	}
}

// TestCreateRootNoAncestorFields verifies a root timeline omits ancestor fields.
func TestCreateRootNoAncestorFields(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, `{"timeline_id":"root"}`)
	}))
	defer srv.Close()

	if _, err := New(srv.URL).CreateBranch("t1", "root", "", "", 16); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["ancestor_timeline_id"]; ok {
		t.Fatal("root timeline must not send ancestor_timeline_id")
	}
	if _, ok := body["ancestor_start_lsn"]; ok {
		t.Fatal("root timeline must not send ancestor_start_lsn")
	}
}

func TestErrorStatusPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"msg":"already exists"}`)
	}))
	defer srv.Close()
	if _, err := New(srv.URL).Timelines("t1"); err == nil {
		t.Fatal("expected error on 409 response")
	}
}

func TestCreateTenantBody(t *testing.T) {
	var body map[string]any
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	if err := New(srv.URL).CreateTenant("t1"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/v1/tenant/t1/location_config" {
		t.Fatalf("got %s %s", gotMethod, gotPath)
	}
	if body["mode"] != "AttachedSingle" {
		t.Fatalf("mode = %v", body["mode"])
	}
}

func TestTenantStateParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"id":"t1","state":{"slug":"Active"}}`)
	}))
	defer srv.Close()
	st, err := New(srv.URL).TenantState("t1")
	if err != nil || st != "Active" {
		t.Fatalf("TenantState = %q, %v; want Active", st, err)
	}
}

func TestWaitTenantActiveSucceeds(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			io.WriteString(w, `{"state":{"slug":"Attaching"}}`)
		} else {
			io.WriteString(w, `{"state":{"slug":"Active"}}`)
		}
	}))
	defer srv.Close()
	if err := New(srv.URL).WaitTenantActive("t1", 10*time.Second); err != nil {
		t.Fatalf("WaitTenantActive: %v", err)
	}
}

func TestWaitTenantActiveTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"state":{"slug":"Attaching"}}`)
	}))
	defer srv.Close()
	if err := New(srv.URL).WaitTenantActive("t1", 1*time.Second); err == nil {
		t.Fatal("expected timeout error")
	}
}

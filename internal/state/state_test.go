package state

import (
	"testing"
)

// useTempHome points the store at a temp dir for the duration of a test.
func useTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("NEON_SELFHOST_HOME", t.TempDir())
}

func TestAddGetRemove(t *testing.T) {
	useTempHome(t)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	const tenant = "t1"
	if err := s.Add(tenant, Branch{Name: "dev", TimelineID: "aaa", Port: 55434}); err != nil {
		t.Fatal(err)
	}
	if b, ok := s.Get(tenant, "dev"); !ok || b.TimelineID != "aaa" {
		t.Fatalf("Get(dev) = %+v, %v", b, ok)
	}
	if b, ok := s.ByTimeline(tenant, "aaa"); !ok || b.Name != "dev" {
		t.Fatalf("ByTimeline(aaa) = %+v, %v", b, ok)
	}
	if !s.Remove(tenant, "dev") {
		t.Fatal("Remove(dev) returned false")
	}
	if _, ok := s.Get(tenant, "dev"); ok {
		t.Fatal("dev still present after Remove")
	}
}

func TestAddDuplicateNameFails(t *testing.T) {
	useTempHome(t)
	s, _ := Load()
	const tenant = "t1"
	if err := s.Add(tenant, Branch{Name: "dev", TimelineID: "aaa"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(tenant, Branch{Name: "dev", TimelineID: "bbb"}); err == nil {
		t.Fatal("expected duplicate name error, got nil")
	}
}

func TestSameNameDifferentTenantsOK(t *testing.T) {
	useTempHome(t)
	s, _ := Load()
	if err := s.Add("t1", Branch{Name: "dev", TimelineID: "aaa"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("t2", Branch{Name: "dev", TimelineID: "bbb"}); err != nil {
		t.Fatalf("same name in different tenant should be allowed: %v", err)
	}
}

func TestAllocPort(t *testing.T) {
	useTempHome(t)
	s, _ := Load()
	// base should be returned first
	p, err := s.AllocPort(55434, 10)
	if err != nil || p != 55434 {
		t.Fatalf("AllocPort empty = %d, %v; want 55434", p, err)
	}
	// occupy 55434 and 55436 across tenants; expect 55435 next
	s.Add("t1", Branch{Name: "a", TimelineID: "1", Port: 55434})
	s.Add("t2", Branch{Name: "b", TimelineID: "2", Port: 55436})
	p, err = s.AllocPort(55434, 10)
	if err != nil || p != 55435 {
		t.Fatalf("AllocPort = %d, %v; want 55435 (lowest free across tenants)", p, err)
	}
}

func TestAllocPortExhausted(t *testing.T) {
	useTempHome(t)
	s, _ := Load()
	s.Add("t1", Branch{Name: "a", TimelineID: "1", Port: 55434})
	s.Add("t1", Branch{Name: "b", TimelineID: "2", Port: 55435})
	if _, err := s.AllocPort(55434, 2); err == nil {
		t.Fatal("expected exhausted-range error, got nil")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	useTempHome(t)
	s, _ := Load()
	s.Add("t1", Branch{Name: "dev", TimelineID: "aaa", Ancestor: "root", Port: 55434})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	b, ok := s2.Get("t1", "dev")
	if !ok || b.Port != 55434 || b.Ancestor != "root" {
		t.Fatalf("round-trip lost data: %+v, %v", b, ok)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	useTempHome(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("Load on missing file should not error: %v", err)
	}
	if len(s.List("whatever")) != 0 {
		t.Fatal("expected empty store")
	}
}

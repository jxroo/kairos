//go:build cgo

package vecbridge

import "testing"

func TestPing(t *testing.T) {
	result := Ping()
	if result != 1 {
		t.Errorf("expected ping=1, got %d", result)
	}
}

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Error("expected non-empty version")
	}
	t.Logf("vecstore version: %s", v)
}

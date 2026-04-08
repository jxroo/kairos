package server

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"go.uber.org/zap"
)

func BenchmarkHealthEndpoint(b *testing.B) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})
	handler := srv.Handler()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkIdleMemory(b *testing.B) {
	logger := zap.NewNop()
	_ = New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	allocMB := float64(m.Alloc) / (1024 * 1024)
	b.ReportMetric(allocMB, "MB-alloc")
	if allocMB > 50 {
		b.Errorf("idle memory usage %.1f MB exceeds 50MB threshold", allocMB)
	}
}

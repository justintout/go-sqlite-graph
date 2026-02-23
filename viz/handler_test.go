package viz_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justintout/go-sqlite-graph/viz"
)

func TestHandler(t *testing.T) {
	c := viz.New(testNodes(), testEdges(), viz.WithTitle("Handler Test"))
	handler := c.Handler()

	req := httptest.NewRequest(http.MethodGet, "/graph", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type text/html; charset=utf-8, got %s", ct)
	}

	if rec.Body.Len() == 0 {
		t.Error("expected non-empty response body")
	}
}

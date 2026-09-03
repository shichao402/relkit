package main

import (
	"net/http"
	"testing"
)

// A 30-minute ReadTimeout once killed a 400 MiB staged upload mid-body: the
// agent answered 400 "read body failed" after exactly 30m and nginx turned the
// dropped upstream into a 502. Uploads must be bounded by size, not by clock.
func TestServerHasNoUploadDeadlines(t *testing.T) {
	srv := newHTTPServer(&Config{Addr: "127.0.0.1:0"}, http.NewServeMux())

	if srv.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %s, want 0 (staged uploads run for hours)", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %s, want 0 (publish signs and uploads before replying)", srv.WriteTimeout)
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout must stay set; it is what bounds a slow header")
	}
	if srv.IdleTimeout <= 0 {
		t.Error("IdleTimeout must stay set so idle keep-alives are reaped")
	}
}

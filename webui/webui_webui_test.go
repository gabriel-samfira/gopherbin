//go:build webui

package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTMLCSPHashesBootstrapScript(t *testing.T) {
	if !strings.Contains(htmlCSP, "script-src 'self' 'sha256-") {
		t.Errorf("htmlCSP missing inline-script hash: %s", htmlCSP)
	}
	scriptSrc := strings.SplitN(strings.Split(htmlCSP, "script-src ")[1], ";", 2)[0]
	if strings.Contains(scriptSrc, "unsafe-inline") {
		t.Errorf("script-src should be hash-based with the real bundle, got: %s", scriptSrc)
	}
}

func TestUIHandlerServesHashedCSPForHTML(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
	w := httptest.NewRecorder()
	UIHandler(w, r)
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "'sha256-") {
		t.Errorf("HTML response CSP missing hash: %q", csp)
	}
}

func TestUIHandlerServesCSPForHTML(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
	w := httptest.NewRecorder()
	UIHandler(w, r)
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors") {
		t.Errorf("HTML response missing CSP: %q", csp)
	}
}

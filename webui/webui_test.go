package webui

import (
	"strings"
	"testing"
)

func TestHTMLCSPIsWellFormed(t *testing.T) {
	for _, d := range []string{"default-src 'self'", "script-src", "frame-ancestors 'none'", "base-uri", "object-src 'none'"} {
		if !strings.Contains(htmlCSP, d) {
			t.Errorf("htmlCSP missing %q: %s", d, htmlCSP)
		}
	}
	scriptSrc := strings.SplitN(strings.Split(htmlCSP, "script-src ")[1], ";", 2)[0]
	if !strings.Contains(scriptSrc, "'self'") {
		t.Errorf("script-src must keep 'self', got %q", scriptSrc)
	}
}

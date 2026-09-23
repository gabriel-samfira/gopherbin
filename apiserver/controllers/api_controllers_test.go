package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gErrors "gopherbin/errors"
)

func TestClampPagination(t *testing.T) {
	cases := []struct {
		name                    string
		page, max, wantP, wantM int64
	}{
		{"defaults when absent", 0, 0, 1, defaultMaxResults},
		{"negative page clamps to 1", -1, 0, 1, defaultMaxResults},
		{"negative max_results clamps to 1 (no unbounded query)", 1, -1, 1, 1},
		{"very negative max_results", 3, -1000000, 3, 1},
		{"max above cap clamps to 100", 2, 5000, 2, maxMaxResults},
		{"in-range values untouched", 4, 25, 4, 25},
		{"boundary max kept", 1, 100, 1, 100},
		{"boundary max min kept", 1, 1, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, m := clampPagination(tc.page, tc.max)
			if p != tc.wantP || m != tc.wantM {
				t.Fatalf("clampPagination(%d, %d) = (%d, %d), want (%d, %d)",
					tc.page, tc.max, p, m, tc.wantP, tc.wantM)
			}
		})
	}
}

func TestSanitizeDownloadName(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "report.txt", "report.txt"},
		{"unicode kept", "été-rapport.txt", "été-rapport.txt"},
		{"quotes and backslashes stripped", `a"b\c.txt`, "abc.txt"},
		{"CRLF injection stripped", "evil\r\nX-Evil: 1", "evilX-Evil: 1"},
		{"control chars stripped", "a\x00\x01\x7fb", "ab"},
		{"non-ASCII format chars stripped", "a\u200b\ufeffb", "ab"},
		{"empty falls back", "   ", "paste"},
		{"only junk falls back", "\"\\", "paste"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeDownloadName(tc.in); got != tc.want {
				t.Fatalf("sanitizeDownloadName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("long name truncated on rune boundary with ellipsis", func(t *testing.T) {
		in := strings.Repeat("é", 500) // multi-byte runes
		got := sanitizeDownloadName(in)
		runes := []rune(got)
		if len(runes) != maxDownloadNameRunes {
			t.Fatalf("len(runes) = %d, want %d", len(runes), maxDownloadNameRunes)
		}
		if runes[maxDownloadNameRunes-1] != '…' {
			t.Fatalf("last rune = %q, want ellipsis", runes[maxDownloadNameRunes-1])
		}
		// No replacement character: truncation never cut a rune in half.
		if strings.ContainsRune(got, '\uFFFD') {
			t.Fatal("truncated name contains U+FFFD: rune boundary violated")
		}
	})
}

func TestContentDisposition(t *testing.T) {
	t.Run("ascii name", func(t *testing.T) {
		got := contentDisposition("report.txt")
		want := `attachment; filename="report.txt"; filename*=UTF-8''report.txt`
		if got != want {
			t.Fatalf("contentDisposition = %q, want %q", got, want)
		}
	})
	t.Run("unicode name gets percent-encoded extended parameter", func(t *testing.T) {
		got := contentDisposition("été.txt")
		want := `attachment; filename="été.txt"; filename*=UTF-8''%C3%A9t%C3%A9.txt`
		if got != want {
			t.Fatalf("contentDisposition = %q, want %q", got, want)
		}
	})
	t.Run("injection attempt cannot break out and is a valid header value", func(t *testing.T) {
		evil := `x"; filename*=evil` + "\r\n" + `X-Injected: 1`
		h := http.Header{}
		h.Set("Content-Disposition", contentDisposition(evil)) // Set() drops invalid values; assert it stuck
		got := h.Get("Content-Disposition")
		if got == "" {
			t.Fatal("header value was rejected by net/http: still contains invalid bytes")
		}
		if strings.ContainsAny(got, "\r\n") {
			t.Fatalf("CRLF survived sanitization: %q", got)
		}
		// The legacy parameter is a single quoted string: exactly two quote
		// characters (open+close), and no backslash inside, so the attacker
		// cannot close the string early. Anything left of their payload
		// (semicolons, asterisks) is inert quoted-string text per RFC 6266.
		if n := strings.Count(got, `"`); n != 2 {
			t.Fatalf("quote count = %d, want exactly 2 (injection broke out): %q", n, got)
		}
		legacy := got[strings.Index(got, `"`) : strings.LastIndex(got, `"`)+1]
		if strings.Contains(legacy, `\`) {
			t.Fatalf("backslash survived in quoted filename: %q", legacy)
		}
		// In the extended (RFC 5987) parameter every separator the attacker
		// used (; " \ *) must be percent-encoded.
		const marker = `"; filename*=UTF-8''`
		idx := strings.Index(got, marker)
		if idx < 0 {
			t.Fatalf("extended parameter missing: %q", got)
		}
		ext := got[idx+len(marker):]
		if strings.ContainsAny(ext, "\"\\\r\n;*") {
			t.Fatalf("unescaped injection characters in filename*: %q", ext)
		}
	})
}

func decode(t *testing.T, w *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var body struct {
		Error   string `json:"error"`
		Details string `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not valid JSON: %v", w.Body.String(), err)
	}
	return body.Error, body.Details
}

func TestHandleErrorInternalLeaksNothing(t *testing.T) {
	internal := fmt.Errorf("running search: %w", fmt.Errorf("fts5: syntax error near \"\\\"\" (no such column: content)"))
	w := httptest.NewRecorder()
	handleError(w, internal)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	errStr, details := decode(t, w)
	if errStr != "Server error" || details != "an internal error occurred" {
		t.Fatalf("body = (%q, %q), want generic server error", errStr, details)
	}
	for _, leak := range []string{"fts5", `syntax error`, "no such column", "running search"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("internal detail %q leaked to client body: %s", leak, w.Body.String())
		}
	}
}

func TestHandleErrorMaxBytesErrorMapsTo413(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"bare", &http.MaxBytesError{}},
		{"wrapped in %w", fmt.Errorf("reading body: %w", &http.MaxBytesError{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleError(w, tc.err)
			if w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d, want 413", w.Code)
			}
			errStr, details := decode(t, w)
			if errStr != "Payload Too Large" || details != "request body exceeds the server limit" {
				t.Fatalf("body = (%q, %q)", errStr, details)
			}
		})
	}
}

func TestHandleErrorTypedBranchesStillEchoOwnDetails(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantStatus  int
		wantErr     string
		wantDetails string
	}{
		{"not found", gErrors.NewNotFoundError("paste abc not found"), http.StatusNotFound, "Not Found", "paste abc not found"},
		{"unauthorized", gErrors.NewUnauthorizedError("invalid username or password"), http.StatusUnauthorized, "Not Authorized", "invalid username or password"},
		{"bad request", gErrors.NewBadRequestError("name too long"), http.StatusBadRequest, "Bad Request", "name too long"},
		{"conflict", gErrors.NewConflictError("team exists"), http.StatusConflict, "Conflict", "team exists"},
		{"duplicate", gErrors.NewDuplicateUserError("duplicate user bob"), http.StatusConflict, "Conflict", "duplicate user bob"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleError(w, tc.err)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			errStr, details := decode(t, w)
			if errStr != tc.wantErr || details != tc.wantDetails {
				t.Fatalf("body = (%q, %q), want (%q, %q)", errStr, details, tc.wantErr, tc.wantDetails)
			}
		})
	}
}

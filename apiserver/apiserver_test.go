// Copyright 2019 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

//go:build fts5

package apiserver

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"gopherbin/config"
)

// probeHandler drains the request body the way the API controllers do (via
// the decode path) and reports what happened: 200 on a clean read, 413 with
// a "maxbytes" marker when the body was cut off by a *http.MaxBytesError
// (i.e. what the shared controller error mapper will surface in production).
func probeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_, _ = w.Write([]byte("maxbytes"))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("readerr"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// testRouter mirrors the production mounting: API routes under /api/v1 on a
// subrouter, everything else on the main router (web UI).
func testRouter() *mux.Router {
	router := mux.NewRouter()
	probe := probeHandler()
	router.PathPrefix("/api/v1").Subrouter().Path("/probe").Handler(probe).Methods(http.MethodPost)
	router.HandleFunc("/probe", probe).Methods(http.MethodPost)
	return router
}

func TestCORS_NoWildcardWhenUnconfigured(t *testing.T) {
	// Empty cors_origins must emit no CORS headers at all (fail closed)
	// instead of gorilla/handlers' wildcard fallback.
	cfg := &config.Config{}
	cfg.APIServer.MaxBodySize = 1024

	req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader("ok"))
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	newServerHandler(testRouter(), cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("want no Access-Control-Allow-Origin, got %q", got)
	}
}

func TestCORS_UnlistedOriginGetsNoHeader(t *testing.T) {
	cfg := &config.Config{}
	cfg.APIServer.MaxBodySize = 1024
	cfg.APIServer.CORSOrigins = []string{"https://app.example"}

	req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader("ok"))
	req.Header.Set("Origin", "https://other.example")
	rec := httptest.NewRecorder()

	newServerHandler(testRouter(), cfg).ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unlisted origin must not get Access-Control-Allow-Origin, got %q", got)
	}
}

func TestCORS_ConfiguredOriginEchoed(t *testing.T) {
	cfg := &config.Config{}
	cfg.APIServer.MaxBodySize = 1024
	cfg.APIServer.CORSOrigins = []string{"https://app.example"}

	req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader("ok"))
	req.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()

	newServerHandler(testRouter(), cfg).ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Errorf("want configured origin echoed, got %q", got)
	}
}

func TestMaxBodySize_APIBodyOverLimitFailsWithMaxBytesError(t *testing.T) {
	cfg := &config.Config{}
	cfg.APIServer.MaxBodySize = 16

	req := httptest.NewRequest(http.MethodPost, "/api/v1/probe", strings.NewReader(strings.Repeat("x", 64)))
	rec := httptest.NewRecorder()

	newServerHandler(testRouter(), cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge || rec.Body.String() != "maxbytes" {
		t.Fatalf("want body cut off with *http.MaxBytesError, got %d %q",
			rec.Code, rec.Body.String())
	}
}

func TestMaxBodySize_APIBodyWithinLimitPasses(t *testing.T) {
	cfg := &config.Config{}
	cfg.APIServer.MaxBodySize = 64

	req := httptest.NewRequest(http.MethodPost, "/api/v1/probe", strings.NewReader(strings.Repeat("x", 32)))
	rec := httptest.NewRecorder()

	newServerHandler(testRouter(), cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body %q)", rec.Code, rec.Body.String())
	}
}

func TestMaxBodySize_NonAPIPathsNotCapped(t *testing.T) {
	// The cap applies only to the /api/v1 chain; the web UI handler
	// must keep seeing full bodies.
	cfg := &config.Config{}
	cfg.APIServer.MaxBodySize = 16

	req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(strings.Repeat("x", 64)))
	rec := httptest.NewRecorder()

	newServerHandler(testRouter(), cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("non-API path must not be capped, got %d (body %q)", rec.Code, rec.Body.String())
	}
}

func TestMaxBodySize_UnsetFallsBackToDefault(t *testing.T) {
	// Zeroed (unvalidated) config must not produce a zero-byte cap.
	cfg := &config.Config{}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/probe", strings.NewReader("hello"))
	rec := httptest.NewRecorder()

	newServerHandler(testRouter(), cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body %q)", rec.Code, rec.Body.String())
	}
}

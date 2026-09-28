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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"

	adminSQL "gopherbin/admin/sql"
	"gopherbin/apiserver/controllers"
	"gopherbin/apiserver/routers"
	"gopherbin/auth"
	"gopherbin/config"
	"gopherbin/params"
	pasteSQL "gopherbin/paste/sql"
)

const contractSecret = "contract-test-secret-0123456789abcdef"

// rxRouteVar matches a gorilla/mux path variable carrying a regex,
// {name:pattern}.
var rxRouteVar = regexp.MustCompile(`\{([A-Za-z]+):([^}]+)\}`)

// routerOperations lists the "METHOD /path" operations the live router
// serves under the API base path, in swagger.yaml's notation: base path
// stripped, trailing slash dropped, and the {name:name\/?} variables (which
// only make a trailing slash optional) written as their literal segment.
func (c *contract) routerOperations() map[string]bool {
	c.t.Helper()
	ops := map[string]bool{}
	err := c.router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tmpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		methods, err := route.GetMethods()
		if err != nil {
			return nil // prefix-only routes (subrouters, catch-alls)
		}
		if tmpl != c.spec.BasePath && !strings.HasPrefix(tmpl, c.spec.BasePath+"/") {
			return nil
		}
		path := rxRouteVar.ReplaceAllStringFunc(strings.TrimPrefix(tmpl, c.spec.BasePath), func(v string) string {
			m := rxRouteVar.FindStringSubmatch(v)
			if m[2] == m[1]+`\/?` {
				return m[1]
			}
			return "{" + m[1] + "}"
		})
		path = strings.TrimSuffix(path, "/")
		for _, method := range methods {
			if method != http.MethodOptions {
				ops[method+" "+path] = true
			}
		}
		return nil
	})
	if err != nil {
		c.t.Fatalf("walking the router: %v", err)
	}
	return ops
}

// specDoc is the slice of the generated swagger.yaml that the contract checks
// need. Only the top level shape of each 200 response matters here: field
// fidelity is structural by construction, because go-swagger derives the
// schemas from the very structs the handlers encode. What this test protects
// is the mapping between routes and models: an endpoint that answers with a
// different type than documented, drops a documented property, is missing
// from the spec, or returns an unexpected status, fails here.
type specDoc struct {
	BasePath string                       `yaml:"basePath"`
	Defs     map[string]specDef           `yaml:"definitions"`
	Paths    map[string]map[string]specOp `yaml:"paths"`
}

type specDef struct {
	Type       string             `yaml:"type"`
	Ref        string             `yaml:"$ref"`
	Items      *specDef           `yaml:"items"`
	GoType     specGoType         `yaml:"x-go-type"`
	Properties map[string]specDef `yaml:"properties"`
	Required   []string           `yaml:"required"`
}

type specGoType struct {
	Type    string `yaml:"type"`
	Package string `yaml:"package"`
}

type specOp struct {
	Responses map[string]specResp `yaml:"responses"`
}

type specResp struct {
	Schema specDef `yaml:"schema"`
}

func loadSpec(t *testing.T) *specDoc {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate apiserver package source")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "swagger.yaml"))
	if err != nil {
		t.Fatalf("reading swagger.yaml (run: go generate ./...): %v", err)
	}
	spec := &specDoc{}
	if err := yaml.Unmarshal(raw, spec); err != nil {
		t.Fatalf("parsing swagger.yaml: %v", err)
	}
	if len(spec.Defs) == 0 || len(spec.Paths) == 0 {
		t.Fatalf("swagger.yaml parsed with %d definitions, %d paths - loader shape is broken", len(spec.Defs), len(spec.Paths))
	}
	return spec
}

// modelFor resolves the documented 200 response of an operation to its top
// level definition. documented == false means the spec has no such operation.
// ok == false means the 200 response carries no object $ref (OK, file or an
// array envelope).
func (s *specDoc) modelFor(path, method string) (name string, def specDef, documented, ok bool) {
	ops, found := s.Paths[path]
	if !found {
		return "", specDef{}, false, false
	}
	op, found := ops[strings.ToLower(method)]
	if !found {
		return "", specDef{}, false, false
	}
	resp, found := op.Responses["200"]
	if !found {
		return "", specDef{}, true, false
	}
	if !strings.HasPrefix(resp.Schema.Ref, "#/definitions/") {
		if (resp.Schema.Type == "object" && len(resp.Schema.Properties) > 0) || resp.Schema.Type == "array" {
			return "(inline)", resp.Schema, true, true
		}
		return "", specDef{}, true, false
	}
	name = strings.TrimPrefix(resp.Schema.Ref, "#/definitions/")
	def, ok = s.Defs[name]
	if !ok {
		return name, specDef{}, true, false
	}
	if len(def.Properties) == 0 && def.Type == "file" {
		return name, specDef{}, true, false
	}
	return name, def, true, true
}

// missingKeys returns the entries of want that are absent from have.
func missingKeys(have, want []string) []string {
	haveSet := map[string]bool{}
	for _, k := range have {
		haveSet[k] = true
	}
	missing := []string{}
	for _, k := range want {
		if !haveSet[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	return missing
}

func jsonKeys(body []byte) ([]string, error) {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// jsonFields returns the JSON names of the fields of a struct type that are
// always present on the wire (no omitempty), flattening embedded structs.
func jsonFields(t reflect.Type) []string {
	names := []string{}
	seen := map[string]bool{}
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				ft := f.Type
				if ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
					walk(ft)
					continue
				}
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]
			if name == "" {
				name = f.Name
			}
			if strings.Contains(tag, "omitempty") {
				continue // value may be absent from the wire
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	walk(t)
	sort.Strings(names)
	return names
}

func ptr[T any](v T) *T { return &v }

type contractUser struct {
	token string
	id    uint
}

type contract struct {
	t          *testing.T
	server     *httptest.Server
	router     *mux.Router
	spec       *specDoc
	admin      contractUser
	member     contractUser
	secondUser contractUser

	// exercised records "METHOD /spec/path" for every operation that has
	// been called at least once; the coverage pass diffs it against the
	// spec.
	exercised map[string]bool
}

// do issues a raw HTTP call against the live router. specPath, when given,
// marks the call as a use of that documented operation and feeds the
// coverage accounting even if the operation is invoked as a fixture (login,
// bootstrap, setup) rather than through invoke/obj/arr/empty.
func (c *contract) do(method, path, token string, body interface{}, specPath ...string) (int, []byte) {
	c.t.Helper()
	for _, sp := range specPath {
		c.exercised[method+" "+sp] = true
	}
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.server.URL+c.spec.BasePath+path, rdr)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.server.Client().Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatalf("reading body of %s %s: %v", method, path, err)
	}
	return res.StatusCode, raw
}

// invoke runs a request against specPath (the route as documented) and
// enforces a 200 on actualPath (the same route with placeholders filled in).
func (c *contract) invoke(method, specPath, actualPath, token string, body interface{}) []byte {
	c.t.Helper()
	c.exercised[method+" "+specPath] = true
	if _, _, documented, _ := c.spec.modelFor(specPath, method); !documented {
		c.t.Errorf("%s %s is not documented in swagger.yaml", method, specPath)
		return nil
	}
	status, raw := c.do(method, actualPath, token, body)
	if status != http.StatusOK {
		c.t.Fatalf("%s %s: got %d, want 200: %s", method, actualPath, status, raw)
	}
	return raw
}

// obj calls a route whose 200 response is an object and validates the JSON
// keys against both the spec model properties and the JSON tags of goType.
// opts carries optional expectations for obj; zero value means defaults.
type opts struct {
	// expect overrides the JSON keys the response must carry (defaults to
	// the Go type's always-present fields).
	expect []string
	// permissiveArray compares array elements against the Go element type
	// instead of demanding that every element match the first element.
	permissiveArray bool
}

func (c *contract) obj(method, specPath, actualPath, token string, body interface{}, goType reflect.Type) []byte {
	return c.objOpts(opts{}, method, specPath, actualPath, token, body, goType)
}

// objOpts is obj with knobs for operations whose wire shape legitimately
// varies between calls.
func (c *contract) objOpts(o opts, method, specPath, actualPath, token string, body interface{}, goType reflect.Type) []byte {
	c.t.Helper()
	name, def, documented, ok := c.spec.modelFor(specPath, method)
	raw := c.invoke(method, specPath, actualPath, token, body)
	if raw == nil || !documented || !ok {
		return raw
	}
	keys, err := jsonKeys(raw)
	if err != nil {
		c.t.Fatalf("%s %s: 200 body is not a JSON object (%v): %s", method, specPath, err, raw)
	}
	if len(def.Properties) == 0 {
		c.t.Errorf("%s %s: spec model %s declares no properties (regenerate swagger.yaml: make generate)", method, specPath, name)
		return raw
	}
	// The spec carries the full property list (apigen inlines it from the
	// Go struct): every key the wire returns must be documented, plus the
	// always-present Go fields and any explicit expectations must be there.
	// Optional spec properties the handler omits are tolerated.
	specKeys := make([]string, 0, len(def.Properties))
	for k := range def.Properties {
		specKeys = append(specKeys, k)
	}
	sort.Strings(specKeys)
	if extra := missingKeys(specKeys, keys); len(extra) > 0 {
		c.t.Errorf("%s %s: response keys %v are not declared by spec model %s", method, specPath, extra, name)
	}
	want := o.expect
	if want == nil && goType != nil {
		want = jsonFields(goType)
	}
	if len(want) > 0 {
		if missing := missingKeys(keys, want); len(missing) > 0 {
			c.t.Errorf("%s %s: response lacks keys %v of %s", method, specPath, missing, name)
		}
	}
	return raw
}

// arr calls a route documented with an array response and checks the body is
// a JSON array whose elements share one object shape.
func (c *contract) arr(method, specPath, actualPath, token string, body interface{}) []byte {
	return c.arrOpts(opts{}, method, specPath, actualPath, token, body)
}

func (c *contract) arrOpts(o opts, method, specPath, actualPath, token string, body interface{}) []byte {
	c.t.Helper()
	raw := c.invoke(method, specPath, actualPath, token, body)
	if raw == nil {
		return nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		c.t.Fatalf("%s %s: 200 body is not a JSON array: %s", method, specPath, raw)
	}
	if name, def, documented, ok := c.spec.modelFor(specPath, method); documented && ok {
		item, itemName := c.spec.arrayItem(def)
		if item == nil {
			c.t.Errorf("%s %s: spec model %s is not an array of objects", method, specPath, name)
		} else {
			for i, elem := range list {
				keys, err := jsonKeys(elem)
				if err != nil {
					c.t.Fatalf("%s %s: element %d is not a JSON object: %s", method, specPath, i, elem)
				}
				specKeys := make([]string, 0, len(item.Properties))
				for k := range item.Properties {
					specKeys = append(specKeys, k)
				}
				if extra := missingKeys(specKeys, keys); len(extra) > 0 {
					c.t.Errorf("%s %s: element %d keys %v are not declared by spec model %s", method, specPath, i, extra, itemName)
				}
				if missing := missingKeys(keys, item.Required); len(missing) > 0 {
					c.t.Errorf("%s %s: element %d lacks required keys %v of %s", method, specPath, i, missing, itemName)
				}
			}
		}
	}
	if o.permissiveArray {
		return raw
	}
	var want []string
	for i, item := range list {
		keys, err := jsonKeys(item)
		if err != nil {
			c.t.Fatalf("%s %s: element %d is not a JSON object: %s", method, specPath, i, item)
		}
		if want == nil {
			want = keys
			continue
		}
		if extra := missingKeys(want, keys); len(extra) > 0 {
			c.t.Errorf("%s %s: element %d lacks keys %v of the first element", method, specPath, i, extra)
		}
		if extra := missingKeys(keys, want); len(extra) > 0 {
			c.t.Errorf("%s %s: element %d has unexpected keys %v beyond the first element", method, specPath, i, extra)
		}
	}
	return raw
}

// empty calls a route documented with the body-less OK response.
func (c *contract) empty(method, specPath, actualPath, token string) {
	c.t.Helper()
	name, _, _, hasRef := c.spec.modelFor(specPath, method)
	if hasRef && name != "OK" {
		c.t.Errorf("%s %s: contract expects the empty OK response, spec says %s", method, specPath, name)
	}
	if raw := c.invoke(method, specPath, actualPath, token, nil); len(bytes.TrimSpace(raw)) > 0 {
		c.t.Errorf("%s %s: documented as body-less, got %s", method, specPath, raw)
	}
}

// arrayItem resolves the item schema of an array response definition.
func (s *specDoc) arrayItem(def specDef) (*specDef, string) {
	if def.Type != "array" || def.Items == nil {
		return nil, ""
	}
	if !strings.HasPrefix(def.Items.Ref, "#/definitions/") {
		if len(def.Items.Properties) == 0 {
			return nil, ""
		}
		return def.Items, "(inline)"
	}
	name := strings.TrimPrefix(def.Items.Ref, "#/definitions/")
	item, ok := s.Defs[name]
	if !ok || len(item.Properties) == 0 {
		return nil, name
	}
	return &item, name
}

// sub runs fn as a subtest and points the helpers at it, so their failures
// (including Fatal) belong to the subtest that made the call.
func (c *contract) sub(name string, fn func(t *testing.T)) {
	parent := c.t
	parent.Run(name, func(t *testing.T) {
		c.t = t
		defer func() { c.t = parent }()
		fn(t)
	})
}

func newContract(t *testing.T) *contract {
	t.Helper()
	spec := loadSpec(t)

	dbCfg := config.Database{
		DbBackend: config.SQLiteBackend,
		SQLite:    config.SQLite{DBFile: filepath.Join(t.TempDir(), "contract.db")},
	}
	paster, err := pasteSQL.NewPaster(dbCfg)
	if err != nil {
		t.Fatalf("NewPaster: %v", err)
	}
	teamMgr, err := pasteSQL.NewTeamManager(dbCfg)
	if err != nil {
		t.Fatalf("NewTeamManager: %v", err)
	}
	userMgr, err := adminSQL.NewUserManager(dbCfg)
	if err != nil {
		t.Fatalf("NewUserManager: %v", err)
	}
	jwtCfg := config.JWTAuth{Secret: contractSecret}

	handler := controllers.NewAPIController(paster, teamMgr, userMgr, jwtCfg, nil)
	jwtMiddleware, err := auth.NewjwtMiddleware(userMgr, jwtCfg)
	if err != nil {
		t.Fatal(err)
	}
	initMiddleware, err := auth.NewInitRequiredMiddleware(userMgr)
	if err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	if err := routers.AddAPIURLs(router, handler, jwtMiddleware, initMiddleware); err != nil {
		t.Fatalf("AddAPIURLs: %v", err)
	}
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	c := &contract{t: t, server: srv, router: router, spec: spec, exercised: map[string]bool{}}

	// Bootstrap: the only superuser can come from /first-run on an empty
	// instance, which doubles as coverage for that endpoint.
	status, raw := c.do(http.MethodPost, "/first-run", "", params.NewUserParams{
		Username: "contractadmin", Email: "admin@contract.test",
		Password: "Kx7#mQ2vLp9!wRt4", FullName: "Contract Admin",
		IsAdmin: true, Enabled: true,
	}, "/first-run")
	if status < 200 || status > 299 {
		t.Fatalf("first-run: got %d: %s", status, raw)
	}
	c.admin = c.login("contractadmin", "Kx7#mQ2vLp9!wRt4")

	for _, seed := range []struct{ name, email, pw string }{
		{"contractmember", "member@contract.test", "Zq8$nvL2tY6#hKd5"},
		{"contractsecond", "second@contract.test", "Brg!4t2xWq7@mnP0"},
	} {
		status, raw = c.do(http.MethodPost, "/admin/users", c.admin.token, params.NewUserParams{
			Username: seed.name, Email: seed.email, Password: seed.pw,
			FullName: "Contract " + seed.name, Enabled: true,
		}, "/admin/users")
		if status < 200 || status > 299 {
			t.Fatalf("seed user %s: got %d: %s", seed.name, status, raw)
		}
	}
	c.member = c.login("contractmember", "Zq8$nvL2tY6#hKd5")
	c.secondUser = c.login("contractsecond", "Brg!4t2xWq7@mnP0")
	return c
}

func (c *contract) login(username, password string) contractUser {
	c.t.Helper()
	status, raw := c.do(http.MethodPost, "/auth/login", "", params.PasswordLoginParams{
		Username: username, Password: password,
	}, "/auth/login")
	if status != http.StatusOK {
		c.t.Fatalf("login as %s: got %d: %s", username, status, raw)
	}
	tok := params.JWTResponse{}
	if err := json.Unmarshal(raw, &tok); err != nil || tok.Token == "" {
		c.t.Fatalf("login as %s returned no token: %s", username, raw)
	}
	status, raw = c.do(http.MethodGet, "/me", tok.Token, nil)
	if status != http.StatusOK {
		c.t.Fatalf("GET /me as %s: %d: %s", username, status, raw)
	}
	me := params.Users{}
	if err := json.Unmarshal(raw, &me); err != nil {
		c.t.Fatal(err)
	}
	return contractUser{token: tok.Token, id: me.ID}
}

// TestAPIContract exercises every documented operation against the real
// router backed by the real SQLite managers, validates the response shapes,
// and diffs spec against exercised routes in both directions.
func TestAPIContract(t *testing.T) {
	c := newContract(t)
	c.runBootstrap()
	c.runAuth()
	c.runPastes()
	c.runLabels()
	c.runUsers()
	c.runTeams()
	c.runAdmin()
	c.runCoverage()
}

func (c *contract) runBootstrap() {
	c.sub("bootstrap", func(t *testing.T) {
		// A bootstrapped instance must refuse further first-runs.
		status, _ := c.do(http.MethodPost, "/first-run/", "", params.NewUserParams{
			Username: "late", Email: "late@contract.test",
			Password: "Kx7#mQ2vLp9!wRt4", FullName: "Late Comer",
		})
		if status != http.StatusConflict {
			t.Errorf("second first-run: got %d, want 409", status)
		}
	})
}

func (c *contract) runAuth() {
	c.sub("auth", func(t *testing.T) {
		c.obj(http.MethodPost, "/auth/login", "/auth/login", "", params.PasswordLoginParams{
			Username: "contractmember", Password: "Zq8$nvL2tY6#hKd5",
		}, reflect.TypeOf(params.JWTResponse{}))

		// A public paste is readable without a token; it is also the
		// fixture for the public view route.
		created := c.obj(http.MethodPost, "/paste", "/paste", c.admin.token, params.Paste{
			Data:     []byte("package main\nfunc main() {}\n"),
			Name:     "public-contract.go",
			Language: "go",
			Public:   true,
		}, reflect.TypeOf(params.Paste{}))
		p := params.Paste{}
		if err := json.Unmarshal(created, &p); err != nil {
			t.Fatal(err)
		}
		c.obj(http.MethodGet, "/public/paste/{pasteID}", "/public/paste/"+p.PasteID, "", nil,
			reflect.TypeOf(params.Paste{}))

		// Authenticated routes must reject anonymous callers.
		if status, _ := c.do(http.MethodGet, "/paste", "", nil); status != http.StatusUnauthorized {
			t.Errorf("GET /paste without token: got %d, want 401", status)
		}
	})
}

func (c *contract) runPastes() {
	c.sub("pastes", func(t *testing.T) {
		created := c.obj(http.MethodPost, "/paste", "/paste", c.admin.token, params.Paste{
			Data:        []byte("contract paste body"),
			Name:        "contract.txt",
			Language:    "text",
			Description: "seeded by the contract test",
			Public:      false,
			Labels:      []params.PasteLabel{{Name: "ci"}},
		}, reflect.TypeOf(params.Paste{}))
		p := params.Paste{}
		if err := json.Unmarshal(created, &p); err != nil {
			t.Fatal(err)
		}

		// Foreign-but-existing pastes answer 404, not 401: "cannot see it"
		// == "does not exist" (no paste-ID enumeration oracle).
		if s, _ := c.do(http.MethodGet, "/paste/"+p.PasteID+"/sharing", c.secondUser.token, nil); s != http.StatusNotFound {
			t.Errorf("GET sharing of foreign paste: got %d, want 404", s)
		}
		if s, _ := c.do(http.MethodDelete, "/paste/"+p.PasteID, c.secondUser.token, nil); s != http.StatusNotFound {
			t.Errorf("DELETE foreign paste: got %d, want 404", s)
		}

		c.obj(http.MethodGet, "/paste", "/paste", c.admin.token, nil, reflect.TypeOf(params.PasteListResult{}))
		c.obj(http.MethodGet, "/paste/{pasteID}", "/paste/"+p.PasteID, c.admin.token, nil,
			reflect.TypeOf(params.Paste{}))
		c.invoke(http.MethodGet, "/paste/{pasteID}/download", "/paste/"+p.PasteID+"/download", c.admin.token, nil)
		c.obj(http.MethodGet, "/paste/search", "/paste/search?q=contract", c.admin.token, nil,
			reflect.TypeOf(params.PasteListResult{}))

		c.obj(http.MethodPut, "/paste/{pasteID}", "/paste/"+p.PasteID, c.admin.token,
			params.UpdatePasteParams{Public: true}, reflect.TypeOf(params.Paste{}))

		c.obj(http.MethodPost, "/paste/{pasteID}/sharing", "/paste/"+p.PasteID+"/sharing", c.admin.token,
			params.UserActionRequest{UserID: "contractmember"}, reflect.TypeOf(params.TeamMember{}))
		c.obj(http.MethodGet, "/paste/{pasteID}/sharing", "/paste/"+p.PasteID+"/sharing", c.admin.token, nil,
			reflect.TypeOf(params.PasteShareListResponse{}))
		c.empty(http.MethodDelete, "/paste/{pasteID}/sharing/{userID}",
			"/paste/"+p.PasteID+"/sharing/contractmember", c.admin.token)

		c.obj(http.MethodPut, "/paste/{pasteID}/labels", "/paste/"+p.PasteID+"/labels", c.admin.token,
			params.PasteLabelsParams{Labels: []string{"ci", "docs"}}, reflect.TypeOf(params.Paste{}))
		c.invoke(http.MethodGet, "/labels/mine", "/labels/mine", c.admin.token, nil)

		c.obj(http.MethodPost, "/paste/{pasteID}/transfer", "/paste/"+p.PasteID+"/transfer", c.admin.token,
			params.UserActionRequest{UserID: "contractmember"}, reflect.TypeOf(params.Paste{}))
		c.empty(http.MethodDelete, "/paste/{pasteID}", "/paste/"+p.PasteID, c.member.token)
	})
}

func (c *contract) runLabels() {
	c.sub("labels", func(t *testing.T) {
		c.obj(http.MethodGet, "/labels", "/labels", c.admin.token, nil, reflect.TypeOf(params.LabelVocabulary{}))
		owned := c.arr(http.MethodGet, "/labels/mine", "/labels/mine", c.admin.token, nil)
		list := []params.LabelInfo{}
		if err := json.Unmarshal(owned, &list); err != nil {
			t.Fatalf("GET /labels/mine is not []LabelInfo: %v", err)
		}
		if len(list) == 0 {
			t.Fatalf("expected the labels created by the paste tests; raw=%s", owned)
		}
		// A foreign user recoloring an existing label gets 404, not 401:
		// small integer label IDs must not be enumerable via the
		// exists(401)/missing(404) difference.
		if s, _ := c.do(http.MethodPut, fmt.Sprintf("/labels/%d", list[0].ID), c.member.token,
			params.UpdateLabelParams{Color: ptr("#00ff00")}); s != http.StatusNotFound {
			t.Errorf("PUT foreign label as member: got %d, want 404", s)
		}
		c.obj(http.MethodPut, "/labels/{labelID}", fmt.Sprintf("/labels/%d", list[0].ID), c.admin.token,
			params.UpdateLabelParams{Color: ptr("#ff0000")}, reflect.TypeOf(params.LabelInfo{}))
		// "docs" belongs to a paste deleted earlier in the run, so removing
		// the label now must succeed and leave "ci" in place.
		docs := list[1]
		for _, l := range list[1:] {
			if l.Name == "docs" {
				docs = l
			}
		}
		c.empty(http.MethodDelete, "/labels/{labelID}", fmt.Sprintf("/labels/%d", docs.ID), c.admin.token)
	})
}

func (c *contract) runUsers() {
	c.sub("users", func(t *testing.T) {
		c.obj(http.MethodGet, "/me", "/me", c.member.token, nil, reflect.TypeOf(params.Users{}))
		c.obj(http.MethodPut, "/me", "/me", c.member.token,
			params.MeSettingsParams{Discoverable: ptr(true)}, reflect.TypeOf(params.Users{}))
		// PUT /me bumps UpdatedAt, which invalidates the token it was made
		// with; log back in so later operations (logout) use a fresh claim.
		c.member = c.login("contractmember", "Zq8$nvL2tY6#hKd5")
		found := c.arr(http.MethodGet, "/users/search", "/users/search?q=contract", c.member.token, nil)
		results := []params.UserSearchResult{}
		if err := json.Unmarshal(found, &results); err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 {
			t.Error("type-ahead found nothing for q=contract")
		}
	})
}

func (c *contract) runTeams() {
	c.sub("teams", func(t *testing.T) {
		c.obj(http.MethodPost, "/teams", "/teams", c.admin.token,
			params.NewTeamParams{Name: "contract-team", Description: "created by the contract test"},
			reflect.TypeOf(params.Teams{}))
		c.obj(http.MethodGet, "/teams", "/teams", c.admin.token, nil, reflect.TypeOf(params.TeamListResult{}))
		c.obj(http.MethodGet, "/teams/{teamName}", "/teams/contract-team", c.admin.token, nil,
			reflect.TypeOf(params.Teams{}))
		c.obj(http.MethodPut, "/teams/{teamName}", "/teams/contract-team", c.admin.token,
			params.UpdateTeamParams{Description: ptr("revised description")}, reflect.TypeOf(params.Teams{}))

		c.obj(http.MethodPost, "/teams/{teamName}/members", "/teams/contract-team/members", c.admin.token,
			params.TeamMemberParams{UserID: "contractmember", Role: "member"}, reflect.TypeOf(params.TeamMember{}))
		c.arrOpts(opts{permissiveArray: true}, http.MethodGet, "/teams/{teamName}/members", "/teams/contract-team/members", c.admin.token, nil)

		// While the invitation is still pending the invitee may view the
		// team, but the roster must carry no contact data for them.
		pvStatus, pvRaw := c.do(http.MethodGet, "/teams/contract-team", c.member.token, nil)
		if pvStatus != http.StatusOK {
			t.Errorf("GET team as pending invitee: got %d, want 200: %s", pvStatus, pvRaw)
		} else {
			pendingView := params.Teams{}
			if err := json.Unmarshal(pvRaw, &pendingView); err != nil {
				t.Fatalf("team view as invitee is not params.Teams: %v", err)
			}
			if pendingView.MyRole != "pending" || pendingView.Owner.Email != "" {
				t.Errorf("pending invitee roster leaks: my_role=%s owner_email=%q", pendingView.MyRole, pendingView.Owner.Email)
			}
			for _, m := range pendingView.Members {
				if m.Email != "" {
					t.Errorf("pending invitee roster leaks email of %s", m.Username)
				}
			}
		}

		c.obj(http.MethodPost, "/teams/{teamName}/accept", "/teams/contract-team/accept", c.member.token, nil,
			reflect.TypeOf(params.Teams{}))
		c.obj(http.MethodPut, "/teams/{teamName}/members/{member}", "/teams/contract-team/members/contractmember",
			c.admin.token, params.SetTeamMemberRoleParams{Role: "admin"}, reflect.TypeOf(params.TeamMember{}))
		c.obj(http.MethodPut, "/teams/{teamName}/labels", "/teams/contract-team/labels", c.admin.token,
			params.TeamLabelsParams{Labels: []string{"ci"}}, reflect.TypeOf(params.Teams{}))

		// A second invite, declined this time, covers /decline plus the
		// invite bell listing.
		c.obj(http.MethodPost, "/teams/{teamName}/members", "/teams/contract-team/members", c.admin.token,
			params.TeamMemberParams{UserID: "contractsecond"}, reflect.TypeOf(params.TeamMember{}))
		c.arr(http.MethodGet, "/teams/invites", "/teams/invites", c.secondUser.token, nil)
		c.empty(http.MethodPost, "/teams/{teamName}/decline", "/teams/contract-team/decline", c.secondUser.token)

		// An ownership offer the owner withdraws, then one the member
		// declines: both actions answer with the team, like accept.
		c.obj(http.MethodPost, "/teams/{teamName}/transfer", "/teams/contract-team/transfer", c.admin.token,
			params.TeamTransferParams{UserID: "contractmember"}, reflect.TypeOf(params.Teams{}))
		c.obj(http.MethodPost, "/teams/{teamName}/transfer/{action}", "/teams/contract-team/transfer/cancel",
			c.admin.token, nil, reflect.TypeOf(params.Teams{}))
		c.obj(http.MethodPost, "/teams/{teamName}/transfer", "/teams/contract-team/transfer", c.admin.token,
			params.TeamTransferParams{UserID: "contractmember"}, reflect.TypeOf(params.Teams{}))
		c.obj(http.MethodPost, "/teams/{teamName}/transfer/{action}", "/teams/contract-team/transfer/decline",
			c.member.token, nil, reflect.TypeOf(params.Teams{}))

		// The owner hands the team over to the member, who accepts.
		c.obj(http.MethodPost, "/teams/{teamName}/transfer", "/teams/contract-team/transfer", c.admin.token,
			params.TeamTransferParams{UserID: "contractmember"}, reflect.TypeOf(params.Teams{}))
		c.arr(http.MethodGet, "/teams/transfers", "/teams/transfers", c.member.token, nil)
		c.obj(http.MethodPost, "/teams/{teamName}/transfer/{action}", "/teams/contract-team/transfer/accept",
			c.member.token, nil, reflect.TypeOf(params.Teams{}))

		// After the handover the former owner is an ordinary member and
		// can leave on their own.
		c.empty(http.MethodPost, "/teams/{teamName}/leave", "/teams/contract-team/leave", c.admin.token)

		// The new owner re-invites the leftover user, removes them again
		// and finally deletes the team.
		c.obj(http.MethodPost, "/teams/{teamName}/members", "/teams/contract-team/members", c.member.token,
			params.TeamMemberParams{UserID: "contractsecond"}, reflect.TypeOf(params.TeamMember{}))
		c.empty(http.MethodDelete, "/teams/{teamName}/members/{member}",
			"/teams/contract-team/members/contractsecond", c.member.token)
		c.empty(http.MethodDelete, "/teams/{teamName}", "/teams/contract-team", c.member.token)

		// Re-create a team so the /labels vocabulary carries a team group
		// when the final assertions run.
		c.obj(http.MethodPost, "/teams", "/teams", c.admin.token,
			params.NewTeamParams{Name: "vocab-team"}, reflect.TypeOf(params.Teams{}))
		// An unrelated user probing an existing team gets 404, not 401:
		// team names are not enumerable through the status difference.
		if s, _ := c.do(http.MethodGet, "/teams/vocab-team", c.secondUser.token, nil); s != http.StatusNotFound {
			t.Errorf("GET foreign team as outsider: got %d, want 404", s)
		}
		c.obj(http.MethodPut, "/teams/{teamName}/labels", "/teams/vocab-team/labels", c.admin.token,
			params.TeamLabelsParams{Labels: []string{"team-label"}}, reflect.TypeOf(params.Teams{}))
		c.obj(http.MethodGet, "/labels", "/labels", c.admin.token, nil, reflect.TypeOf(params.LabelVocabulary{}))
	})
}

func (c *contract) runAdmin() {
	c.sub("admin", func(t *testing.T) {
		c.obj(http.MethodGet, "/admin/users", "/admin/users", c.admin.token, nil, reflect.TypeOf(params.UserListResult{}))
		c.obj(http.MethodGet, "/admin/users/{userID}", fmt.Sprintf("/admin/users/%d", c.member.id), c.admin.token, nil,
			reflect.TypeOf(params.Users{}))
		c.obj(http.MethodPut, "/admin/users/{userID}", fmt.Sprintf("/admin/users/%d", c.secondUser.id), c.admin.token,
			params.UpdateUserPayload{FullName: ptr("Renamed Second")}, reflect.TypeOf(params.Users{}))

		// A username collision must surface as 409, not leak the unique index
		// violation as a 500.
		if dstatus, draw := c.do(http.MethodPost, "/admin/users", c.admin.token, params.NewUserParams{
			Username: "contractsecond", Email: "duplicate@contract.local",
			FullName: "Collision", Password: "Xk9!mnQ2vLp7wRt4", Enabled: true,
		}); dstatus != http.StatusConflict {
			t.Errorf("POST /admin/users with duplicate username: got %d, want 409: %s", dstatus, draw)
		}

		// The member is not an admin and must be refused. GopherBin answers
		// permission failures on this surface with 401.
		c.member = c.login("contractmember", "Zq8$nvL2tY6#hKd5")
		if mstatus, _ := c.do(http.MethodGet, "/admin/users", c.member.token, nil); mstatus != http.StatusUnauthorized {
			t.Errorf("GET /admin/users as member: got %d, want 401", mstatus)
		}

		// Undocumented but served: GET /logout blacklists the bearer's
		// token; afterwards the token must be refused.
		// Refresh the token: an earlier PUT /admin/users bumped the user's
		// UpdatedAt, which invalidates claims minted before it.
		c.secondUser = c.login("contractsecond", "Brg!4t2xWq7@mnP0")
		c.exercised["GET /logout"] = true
		lstatus, lraw := c.do(http.MethodGet, "/logout", c.secondUser.token, nil)
		if lstatus != http.StatusOK {
			t.Errorf("GET /logout: got %d: %s", lstatus, lraw)
		}
		bstatus, braw := c.do(http.MethodGet, "/me", c.secondUser.token, nil)
		if bstatus != http.StatusUnauthorized {
			t.Errorf("GET /me after logout: got %d: %s", bstatus, braw)
		}
		c.empty(http.MethodDelete, "/admin/users/{userID}", fmt.Sprintf("/admin/users/%d", c.secondUser.id), c.admin.token)
	})
}

// runCoverage fails when the spec, the router and the calls made above
// disagree about the API surface, in any direction.
func (c *contract) runCoverage() {
	c.sub("coverage", func(t *testing.T) {
		documented := map[string]bool{}
		for path, ops := range c.spec.Paths {
			for method := range ops {
				documented[strings.ToUpper(method)+" "+path] = true
			}
		}
		for op := range c.exercised {
			if !documented[op] {
				t.Errorf("exercised %q but swagger.yaml does not document it", op)
			}
		}
		for op := range documented {
			if !c.exercised[op] {
				t.Errorf("swagger.yaml documents %q but the contract test never called it", op)
			}
		}

		// The live router is the authority on what the API serves.
		served := c.routerOperations()
		for op := range served {
			if !documented[op] {
				t.Errorf("router serves %q but swagger.yaml does not document it", op)
			}
		}
		for op := range documented {
			if !served[op] {
				t.Errorf("swagger.yaml documents %q but the router does not serve it", op)
			}
		}
	})
}

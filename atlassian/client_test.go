package atlassian

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, product Product, h http.Handler, opts ...Option) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(product, srv.URL, "me@example.com", "secret", "approvekit-test/1", opts...)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c, srv
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestRequestHeadersAndDecoding(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, Jira, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("me@example.com:secret"))
		if r.Header.Get("Authorization") != wantAuth || r.Header.Get("User-Agent") != "approvekit-test/1" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("headers = %v", r.Header)
		}
		if r.Method == http.MethodPost && r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		writeJSON(w, 200, map[string]string{"accountId": "acc-me", "q": r.URL.Query().Get("q")})
	}))
	var out struct {
		AccountID string `json:"accountId"`
		Q         string `json:"q"`
	}
	if err := c.Do(context.Background(), http.MethodPost, "/rest/api/3/myself", map[string][]string{"q": {"x y"}}, map[string]int{"a": 1}, &out); err != nil || out.AccountID != "acc-me" || out.Q != "x y" {
		t.Fatalf("Do = %+v, %v", out, err)
	}
}

func TestErrorMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		product Product
		status  int
		body    string
		want    string
		is      error
	}{
		{Jira, 401, ``, "", ErrUnauthorized},
		{Jira, 401, `{"code":401,"message":"Unauthorized; scope does not match"}`, "", ErrMissingScope},
		{Confluence, 404, `{"errors":[{"title":"Not Found"}]}`, "", ErrNotFound},
		{Jira, 403, `{"errorMessages":["You do not have permission"]}`, "permission denied by Jira: You do not have permission", nil},
		{Jira, 400, `{"errorMessages":["bad"],"errors":{"summary":"required","labels":"invalid"}}`, "Jira returned HTTP 400: bad; labels: invalid; summary: required", nil},
		{Confluence, 400, `{"errors":[{"status":400,"code":"INVALID_REQUEST_PARAMETER","title":"Provided value {x} for 'id' is not the correct type.","detail":null}]}`, "Confluence returned HTTP 400: Provided value {x} for 'id' is not the correct type.", nil},
		{Confluence, 409, `{"statusCode":409,"message":"Version must be incremented on update."}`, "Confluence returned HTTP 409: Version must be incremented on update.", nil},
		{Confluence, 500, `not json`, "Confluence returned HTTP 500", nil},
	}
	for _, tt := range tests {
		c, _ := newTestClient(t, tt.product, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
			_, _ = io.WriteString(w, tt.body)
		}))
		err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
		switch {
		case err == nil:
			t.Errorf("%d %s: no error", tt.status, tt.body)
		case tt.is != nil && !errors.Is(err, tt.is):
			t.Errorf("%d %s: err = %v, want %v", tt.status, tt.body, err, tt.is)
		case tt.want != "" && err.Error() != tt.want:
			t.Errorf("%d %s: err = %q, want %q", tt.status, tt.body, err, tt.want)
		}
	}
}

func TestMissingScopeHint(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, Confluence, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 401, map[string]any{"code": 401, "message": "Unauthorized; scope does not match"})
	}), WithScopeHint(func(method, path string) string {
		if method == http.MethodGet && path == "/wiki/api/v2/spaces" {
			return "read:space:confluence"
		}
		return ""
	}))
	err := c.Do(context.Background(), http.MethodGet, "/wiki/api/v2/spaces", nil, nil, nil)
	if !errors.Is(err, ErrMissingScope) || !strings.Contains(err.Error(), "needs read:space:confluence") {
		t.Errorf("err = %v", err)
	}
}

func TestGatewayRouting(t *testing.T) {
	t.Parallel()
	const cloud = "0b8e7c1a-5d2f-4e3a-9b6c-1f2e3d4c5b6a"
	for _, product := range []Product{Jira, Confluence} {
		var path atomic.Value
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path.Store(r.URL.Path)
			writeJSON(w, 200, map[string]string{})
		}))
		c := New(product, "https://team.atlassian.net", "me@example.com", "secret", "t", WithGateway(srv.URL, cloud))
		if err := c.Do(context.Background(), http.MethodGet, "/rest/x", nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		if want := "/ex/" + string(product) + "/" + cloud + "/rest/x"; path.Load() != want {
			t.Errorf("%s: path = %v, want %s", product, path.Load(), want)
		}
		if c.Site() != "https://team.atlassian.net" {
			t.Errorf("site = %s", c.Site())
		}
		srv.Close()
	}
}

func TestCloudID(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, Jira, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_edge/tenant_info" || r.Header.Get("Authorization") != "" {
			t.Errorf("path %s, auth %q: the lookup must be unauthenticated", r.URL.Path, r.Header.Get("Authorization"))
		}
		writeJSON(w, 200, map[string]string{"cloudId": "0b8e7c1a-5d2f-4e3a-9b6c-1f2e3d4c5b6a"})
	}))
	if id, err := c.CloudID(context.Background()); err != nil || id != "0b8e7c1a-5d2f-4e3a-9b6c-1f2e3d4c5b6a" {
		t.Errorf("CloudID = %q, %v", id, err)
	}
	bad, _ := newTestClient(t, Jira, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"cloudId": "../../evil"})
	}))
	if _, err := bad.CloudID(context.Background()); err == nil {
		t.Error("invalid cloud ID accepted")
	}
}

func TestRetriesRateLimit(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var waited []time.Duration
	c, _ := newTestClient(t, Jira, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, 200, map[string]string{})
	}))
	c.sleep = func(_ context.Context, d time.Duration) error { waited = append(waited, d); return nil }
	if err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || len(waited) != 2 || waited[0] != 2*time.Second {
		t.Errorf("calls = %d waited = %v", calls.Load(), waited)
	}
}

func TestGivesUpAfterRetries(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	c, _ := newTestClient(t, Jira, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	if err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != maxRetries+1 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestRefusesCrossHostRedirect(t *testing.T) {
	t.Parallel()
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	t.Cleanup(other.Close)
	c, _ := newTestClient(t, Jira, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1), http.StatusFound)
	}))
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "refusing redirect") {
		t.Errorf("err = %v", err)
	}
	if leaked.Load() {
		t.Error("request reached the other host")
	}
}

func TestMultipartUpload(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t, Confluence, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Atlassian-Token") != "no-check" {
			t.Error("upload without X-Atlassian-Token")
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(f)
		writeJSON(w, 200, map[string]string{"name": h.Filename, "data": string(data)})
	}))
	body, err := Multipart("file", "notes.txt", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := c.Do(context.Background(), http.MethodPost, "/upload", nil, body, &out); err != nil || out["name"] != "notes.txt" || out["data"] != "hello" {
		t.Errorf("upload = %v, %v", out, err)
	}
}

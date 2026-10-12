// Package atlassian is the HTTP core for Atlassian Cloud REST APIs: basic
// authentication with an API token, routing through Atlassian's API gateway
// for scoped tokens, retries on rate limits, error decoding for Jira and
// Confluence, and multipart uploads. Product endpoints are built on Do and
// Send by the tools themselves.
package atlassian

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Product selects the API family behind the gateway.
type Product string

const (
	Jira       Product = "jira"
	Confluence Product = "confluence"
)

func (p Product) name() string {
	switch p {
	case Jira:
		return "Jira"
	case Confluence:
		return "Confluence"
	}
	return "Atlassian"
}

// Gateway is Atlassian's API gateway, the only host that accepts scoped API
// tokens.
const Gateway = "https://api.atlassian.com"

const (
	maxJSONBytes = 16 << 20
	maxRetries   = 3
	maxRetryWait = 10 * time.Second
)

var (
	// ErrUnauthorized means the credentials were rejected.
	ErrUnauthorized = errors.New("credentials rejected: the API token may be expired or revoked; create a new one at https://id.atlassian.com/manage-profile/security/api-tokens")
	// ErrNotFound covers missing resources and resources the user may not see.
	ErrNotFound = errors.New("not found, or you do not have permission to see it")
	// ErrMissingScope means a scoped API token lacks a scope the request needs.
	ErrMissingScope = errors.New("the API token lacks a scope this request needs")
)

var cloudIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// APIError is a non-2xx response that is not mapped to a sentinel error.
type APIError struct {
	Product  Product
	Status   int
	Messages []string
}

func (e *APIError) Error() string {
	prefix := fmt.Sprintf("%s returned HTTP %d", e.Product.name(), e.Status)
	if e.Status == http.StatusForbidden {
		prefix = "permission denied by " + e.Product.name()
	}
	if len(e.Messages) == 0 {
		return prefix
	}
	return prefix + ": " + strings.Join(e.Messages, "; ")
}

// Client talks to one Atlassian Cloud site as one user.
type Client struct {
	product   Product
	site      string
	base      string // where API requests go: the site, or the gateway
	auth      string
	userAgent string
	http      *http.Client
	sleep     func(context.Context, time.Duration) error
	scopeHint func(method, path string) string
}

// Option customises a Client.
type Option func(*Client)

// WithGateway sends API requests through gateway (normally Gateway) for the
// site with cloudID, as scoped API tokens require.
func WithGateway(gateway, cloudID string) Option {
	return func(c *Client) {
		c.base = strings.TrimRight(gateway, "/") + "/ex/" + string(c.product) + "/" + cloudID
	}
}

// WithTransport sets the HTTP transport, e.g. to trust a test server.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *Client) { c.http.Transport = rt }
}

// WithScopeHint names the scopes a request needs, for the error when a
// scoped token lacks one. hint returns "" when it does not know.
func WithScopeHint(hint func(method, path string) string) Option {
	return func(c *Client) { c.scopeHint = hint }
}

// New returns a client for site (already normalised to https://host).
// userAgent identifies the tool, such as "tix-for-jira/v0.2.0".
func New(product Product, site, email, token, userAgent string, opts ...Option) *Client {
	c := &Client{
		product:   product,
		site:      strings.TrimRight(site, "/"),
		auth:      "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
		userAgent: userAgent,
		http: &http.Client{
			Timeout:       60 * time.Second,
			CheckRedirect: sameOrigin,
		},
		sleep: sleepCtx,
	}
	c.base = c.site
	for _, o := range opts {
		o(c)
	}
	return c
}

// Site returns the site URL, e.g. https://team.atlassian.net, for links.
func (c *Client) Site() string { return c.site }

// CloudID looks up the site's cloud ID, which Atlassian publishes without
// authentication.
func (c *Client) CloudID(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.site+"/_edge/tenant_info", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("looking up the cloud ID of %s: HTTP %d", c.site, resp.StatusCode)
	}
	var info struct {
		CloudID string `json:"cloudId"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&info); err != nil {
		return "", fmt.Errorf("looking up the cloud ID of %s: %w", c.site, err)
	}
	if !cloudIDRe.MatchString(info.CloudID) {
		return "", fmt.Errorf("%s returned an invalid cloud ID", c.site)
	}
	return info.CloudID, nil
}

// sameOrigin refuses redirects that change scheme or host, so the token
// never leaves the site or the gateway.
func sameOrigin(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != via[0].URL.Scheme || req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("refusing redirect to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RawBody is a request body sent as is, such as a multipart upload. It is
// sent with the header Atlassian requires for uploads.
type RawBody struct {
	Data        []byte
	ContentType string
}

// Multipart builds an upload body with one file, as the attachment
// endpoints of Jira and Confluence expect.
func Multipart(field, filename string, data []byte) (RawBody, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		return RawBody{}, err
	}
	if _, err := part.Write(data); err != nil {
		return RawBody{}, err
	}
	if err := w.Close(); err != nil {
		return RawBody{}, err
	}
	return RawBody{Data: buf.Bytes(), ContentType: w.FormDataContentType()}, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var r io.Reader
	contentType := "application/json"
	switch b := body.(type) {
	case nil:
	case RawBody:
		r, contentType = bytes.NewReader(b.Data), b.ContentType
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if _, ok := body.(RawBody); ok {
		// Uploads are refused without it (XSRF protection).
		req.Header.Set("X-Atlassian-Token", "no-check")
	}
	return req, nil
}

// Send performs the request, retrying on 429 and 503 as advised by
// Retry-After. The caller must close the body of a successful response.
func (c *Client) Send(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := c.newRequest(ctx, method, path, query, body)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable
		if retryable && attempt < maxRetries {
			wait := retryAfter(resp.Header.Get("Retry-After"), attempt)
			_ = resp.Body.Close()
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}
		defer func() { _ = resp.Body.Close() }()
		err = c.decodeError(resp)
		if errors.Is(err, ErrMissingScope) && c.scopeHint != nil {
			if scopes := c.scopeHint(method, path); scopes != "" {
				err = fmt.Errorf("%w (this request needs %s)", err, scopes)
			}
		}
		return nil, err
	}
}

// Do sends a request and decodes a JSON response into out, if out is not
// nil. body is marshalled as JSON unless it is a RawBody.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	resp, err := c.Send(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxJSONBytes))
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxJSONBytes {
		return fmt.Errorf("response from %s exceeds %d bytes", path, maxJSONBytes)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding response from %s: %w", path, err)
	}
	return nil
}

func retryAfter(h string, attempt int) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && s >= 0 {
		return min(time.Duration(s)*time.Second, maxRetryWait)
	}
	return min(time.Duration(1<<attempt)*time.Second, maxRetryWait)
}

func (c *Client) decodeError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		// The gateway answers a missing scope with 401 and this message.
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		if bytes.Contains(b, []byte("scope does not match")) {
			return ErrMissingScope
		}
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return &APIError{Product: c.product, Status: resp.StatusCode, Messages: errorMessages(data)}
}

// errorMessages reads the error formats of Jira ({"errorMessages": [...],
// "errors": {"field": "message"}}), the Confluence v2 API ({"errors":
// [{"title": ..., "detail": ...}]}) and the Confluence v1 API
// ({"message": ...}).
func errorMessages(data []byte) []string {
	var body struct {
		ErrorMessages []string        `json:"errorMessages"`
		Errors        json.RawMessage `json:"errors"`
		Message       string          `json:"message"`
	}
	if json.Unmarshal(data, &body) != nil {
		return nil
	}
	msgs := append([]string(nil), body.ErrorMessages...)
	var fields map[string]string
	var list []struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	switch {
	case json.Unmarshal(body.Errors, &fields) == nil:
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			msgs = append(msgs, k+": "+fields[k])
		}
	case json.Unmarshal(body.Errors, &list) == nil:
		for _, e := range list {
			m := e.Title
			if e.Detail != "" && e.Detail != e.Title {
				m = strings.TrimSpace(m + " " + e.Detail)
			}
			if m != "" {
				msgs = append(msgs, m)
			}
		}
	}
	if len(msgs) == 0 && body.Message != "" {
		msgs = append(msgs, body.Message)
	}
	return msgs
}

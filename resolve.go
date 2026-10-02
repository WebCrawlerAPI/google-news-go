// Package googlenews resolves news.google.com article links to publisher URLs.
//
// It uses an undocumented Google News RPC, so it may break if Google changes it.
package googlenews

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxBodyBytes = 5_000_000

var (
	pathRe  = regexp.MustCompile(`^/(?:read|articles|rss/articles)/([A-Za-z0-9_-]+)/?$`)
	tagRe   = regexp.MustCompile(`<[^>]*\bdata-n-a-id=[^>]*>`)
	attrRe  = regexp.MustCompile(`\bdata-n-a-(id|ts|sg)=["']([^"']*)["']`)
	digitRe = regexp.MustCompile(`^\d+$`)

	// HTTPClient is used for all requests; replace it to set proxies or timeouts.
	HTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// Result is the resolved publisher URL plus diagnostics.
type Result struct {
	URL       string `json:"url"`
	Method    string `json:"method"`
	HTMLBytes int    `json:"htmlBytes"`
	RPCBytes  int    `json:"rpcBytes,omitempty"`
}

type fetched struct {
	text  string
	bytes int
	url   *url.URL
}

// fetchText caps the body size; Google's scripts are never evaluated.
func fetchText(ctx context.Context, method, target string, form url.Values) (*fetched, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, resp.Request.URL.Hostname())
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBodyBytes {
		return nil, errors.New("Response exceeds 5 MB")
	}
	return &fetched{text: string(data), bytes: len(data), url: resp.Request.URL}, nil
}

// Resolve resolves a news.google.com article URL to the publisher URL.
func Resolve(ctx context.Context, input string) (*Result, error) {
	source, err := url.Parse(input)
	if err != nil {
		return nil, err
	}
	match := pathRe.FindStringSubmatch(source.Path)
	if source.Scheme != "https" || source.Hostname() != "news.google.com" || match == nil {
		return nil, errors.New("Expected an HTTPS news.google.com read/articles/rss/articles URL")
	}
	id := match[1]

	// The RSS wrapper returns roughly half as much HTML as /read.
	q := url.Values{"hl": {"en-US"}, "gl": {"US"}, "ceid": {"US:en"}}
	pageURL := "https://news.google.com/rss/articles/" + id + "?" + q.Encode()
	page, err := fetchText(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	host := page.url.Hostname()
	if host != "news.google.com" && !strings.HasSuffix(host, ".google.com") {
		return &Result{URL: page.url.String(), Method: "http-redirect", HTMLBytes: page.bytes}, nil
	}

	// Match attributes on one element, not arbitrary URLs in the large JS bundle.
	var attrs map[string]string
	for _, tag := range tagRe.FindAllString(page.text, -1) {
		m := map[string]string{}
		for _, a := range attrRe.FindAllStringSubmatch(tag, -1) {
			m[a[1]] = a[2]
		}
		if m["id"] == id {
			attrs = m
			break
		}
	}
	if attrs == nil || attrs["sg"] == "" || !digitRe.MatchString(attrs["ts"]) {
		return nil, errors.New("Missing Google News signature/timestamp (consent, blocking, or markup change)")
	}
	ts, err := strconv.ParseInt(attrs["ts"], 10, 64)
	if err != nil {
		return nil, err
	}

	// Undocumented Google News RPC context; keep the tested wire format intact.
	reqContext := []any{
		[]any{"en-US", "US", []any{"FINANCE_TOP_INDICES", "WEB_TEST_1_0_0"}, nil, nil,
			1, 1, "US:en", nil, 180, nil, nil, nil, nil, nil, 0, nil, nil,
			[]any{1608992183, 723341000}},
		"en-US", "US", 1, []any{2, 3, 4, 8}, 1, 0, "655000234", 0, 0, nil, 0,
	}
	args, err := marshal([]any{"garturlreq", reqContext, id, ts, attrs["sg"]})
	if err != nil {
		return nil, err
	}
	freq, err := marshal([]any{[]any{[]any{"Fbv4je", args, nil, "generic"}}})
	if err != nil {
		return nil, err
	}
	rpc, err := fetchText(ctx, http.MethodPost,
		"https://news.google.com/_/DotsSplashUi/data/batchexecute?rpcids=Fbv4je",
		url.Values{"f.req": {freq}})
	if err != nil {
		return nil, err
	}

	// Skip the anti-XSSI prefix and byte-count lines; parse JSON twice.
	for _, line := range strings.Split(rpc.text, "\n") {
		if !strings.HasPrefix(line, "[[") {
			continue
		}
		var rows [][]any
		if err := json.Unmarshal([]byte(line), &rows); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if len(row) < 3 || row[0] != "wrb.fr" || row[1] != "Fbv4je" {
				continue
			}
			payload, ok := row[2].(string)
			if !ok {
				continue
			}
			var result []any
			if err := json.Unmarshal([]byte(payload), &result); err != nil {
				return nil, err
			}
			if len(result) < 2 || result[0] != "garturlres" {
				continue
			}
			dest, ok := result[1].(string)
			if !ok {
				continue
			}
			u, err := url.Parse(dest)
			if err != nil {
				return nil, err
			}
			if u.Scheme != "https" && u.Scheme != "http" {
				return nil, errors.New("Unexpected destination protocol")
			}
			return &Result{URL: u.String(), Method: "google-news-rpc", HTMLBytes: page.bytes, RPCBytes: rpc.bytes}, nil
		}
	}
	return nil, errors.New("Google News RPC returned no article URL (HTTP 200 alone does not mean success)")
}

// marshal is json.Marshal without HTML escaping, matching JS JSON.stringify.
func marshal(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

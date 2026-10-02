package googlenews

import (
	"context"
	"testing"
)

func TestResolveRejectsBadURLs(t *testing.T) {
	for _, in := range []string{
		"http://news.google.com/read/abc",
		"https://example.com/read/abc",
		"https://news.google.com/foo/abc",
		"https://news.google.com/read/",
	} {
		if _, err := Resolve(context.Background(), in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

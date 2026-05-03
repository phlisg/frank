package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type mockTransport struct {
	handler func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.handler(req)
}

func mockClient(statusCode int, body string) *http.Client {
	return &http.Client{
		Transport: &mockTransport{
			handler: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: statusCode,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			},
		},
	}
}

func mockClientError() *http.Client {
	return &http.Client{
		Transport: &mockTransport{
			handler: func(_ *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("network error")
			},
		},
	}
}

func writeCacheFile(t *testing.T, ts int64, version string) string {
	t.Helper()
	path := cacheFilePath()
	content := fmt.Sprintf("%d\n%s\n", ts, version)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestCheck_CacheHit(t *testing.T) {
	called := false
	orig := client
	client = &http.Client{
		Transport: &mockTransport{
			handler: func(_ *http.Request) (*http.Response, error) {
				called = true
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v2.0.0"}`)),
				}, nil
			},
		},
	}
	defer func() { client = orig }()

	writeCacheFile(t, time.Now().Unix(), "1.5.0")

	status, err := Check("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("expected no HTTP call when cache is fresh")
	}
	if !status.Available {
		t.Error("expected Available=true")
	}
	if status.Latest != "1.5.0" {
		t.Errorf("expected Latest=1.5.0, got %s", status.Latest)
	}
}

func TestCheck_CacheExpired(t *testing.T) {
	orig := client
	client = mockClient(200, `{"tag_name":"v2.0.0"}`)
	defer func() { client = orig }()

	staleTS := time.Now().Add(-20 * time.Minute).Unix()
	writeCacheFile(t, staleTS, "1.0.0")

	status, err := Check("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available {
		t.Error("expected Available=true")
	}
	if status.Latest != "2.0.0" {
		t.Errorf("expected Latest=2.0.0, got %s", status.Latest)
	}
}

func TestCheck_CacheMiss(t *testing.T) {
	orig := client
	client = mockClient(200, `{"tag_name":"v1.3.0"}`)
	defer func() { client = orig }()

	// Ensure no cache file exists
	os.Remove(cacheFilePath())
	t.Cleanup(func() { os.Remove(cacheFilePath()) })

	status, err := Check("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available {
		t.Error("expected Available=true")
	}
	if status.Latest != "1.3.0" {
		t.Errorf("expected Latest=1.3.0, got %s", status.Latest)
	}
}

func TestCheck_SameVersion(t *testing.T) {
	orig := client
	client = mockClient(200, `{"tag_name":"v1.0.0"}`)
	defer func() { client = orig }()

	os.Remove(cacheFilePath())
	t.Cleanup(func() { os.Remove(cacheFilePath()) })

	status, err := Check("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if status.Available {
		t.Error("expected Available=false for same version")
	}
}

func TestCheck_APIFailure(t *testing.T) {
	orig := client
	client = mockClientError()
	defer func() { client = orig }()

	os.Remove(cacheFilePath())

	status, err := Check("1.0.0")
	if err != nil {
		t.Error("expected no error on API failure")
	}
	if status.Available {
		t.Error("expected Available=false on API failure")
	}
}

func TestCheck_CorruptCache(t *testing.T) {
	orig := client
	client = mockClient(200, `{"tag_name":"v3.0.0"}`)
	defer func() { client = orig }()

	// Write corrupt cache
	path := cacheFilePath()
	if err := os.WriteFile(path, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })

	status, err := Check("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available {
		t.Error("expected Available=true after refetch")
	}
	if status.Latest != "3.0.0" {
		t.Errorf("expected Latest=3.0.0, got %s", status.Latest)
	}
}

func TestCheck_NewerCurrent(t *testing.T) {
	orig := client
	client = mockClient(200, `{"tag_name":"v1.0.0"}`)
	defer func() { client = orig }()

	os.Remove(cacheFilePath())
	t.Cleanup(func() { os.Remove(cacheFilePath()) })

	status, err := Check("2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if status.Available {
		t.Error("expected Available=false when current is newer")
	}
}

// Ensure UID is part of cache file path
func TestCacheFilePath_ContainsUID(t *testing.T) {
	path := cacheFilePath()
	uid := strconv.Itoa(os.Getuid())
	if !strings.Contains(path, uid) {
		t.Errorf("cache path %q does not contain UID %s", path, uid)
	}
}

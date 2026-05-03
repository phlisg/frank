package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Status reports whether an update is available.
type Status struct {
	Available bool
	Latest    string
	Method    Method
}

type httpClient interface {
	Get(url string) (*http.Response, error)
}

var client httpClient = http.DefaultClient

const (
	cacheTTL = 15 * time.Minute
	apiURL   = "https://api.github.com/repos/phlisg/frank/releases/latest"
)

// Check determines if a newer version of frank is available.
func Check(currentVersion string) (Status, error) {
	cacheFile := cacheFilePath()

	if latest, ok := readCache(cacheFile); ok {
		return compareVersions(currentVersion, latest), nil
	}

	latest, err := fetchLatest()
	if err != nil {
		return Status{Available: false, Method: DetectMethod()}, nil
	}

	writeCache(cacheFile, latest)
	return compareVersions(currentVersion, latest), nil
}

func cacheFilePath() string {
	uid := strconv.Itoa(os.Getuid())
	return fmt.Sprintf("%s/frank-update-check-%s", os.TempDir(), uid)
}

func readCache(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) != 2 {
		return "", false
	}

	ts, err := strconv.ParseInt(lines[0], 10, 64)
	if err != nil {
		return "", false
	}

	cached := time.Unix(ts, 0)
	if time.Since(cached) >= cacheTTL {
		return "", false
	}

	return lines[1], true
}

func writeCache(path, version string) {
	content := fmt.Sprintf("%d\n%s\n", time.Now().Unix(), version)
	_ = os.WriteFile(path, []byte(content), 0o644)
}

func fetchLatest() (string, error) {
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func compareVersions(current, latest string) Status {
	s := Status{
		Latest: latest,
		Method: DetectMethod(),
	}

	vc := "v" + strings.TrimPrefix(current, "v")
	vl := "v" + strings.TrimPrefix(latest, "v")

	if semver.IsValid(vc) && semver.IsValid(vl) && semver.Compare(vl, vc) > 0 {
		s.Available = true
	}

	return s
}

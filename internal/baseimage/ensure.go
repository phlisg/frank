package baseimage

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/output"
	"github.com/phlisg/frank/internal/template"
)

// labelKey is the image label that carries the rendered base Dockerfile's
// content hash. ensure-base compares it against the freshly computed hash to
// decide whether the shared base image is stale.
const labelKey = "frank.base.hash"

// needsBuild reports whether the base image must be (re)built.
//
//   - present=false (image absent) -> true.
//   - present=true but gotLabel != wantHash (template-body drift, or label
//     missing/"<no value>") -> true.
//   - present=true and gotLabel == wantHash -> false (skip, instant).
func needsBuild(present bool, gotLabel, wantHash string) bool {
	if !present {
		return true
	}

	gotLabel = strings.TrimSpace(gotLabel)
	if gotLabel == "" || gotLabel == "<no value>" {
		return true
	}

	return gotLabel != wantHash
}

// EnsureBase guarantees the shared base runtime image for cfg exists and is
// fresh, rebuilding it once (under a host file lock so concurrent frank
// invocations serialize) when absent or drifted.
//
// This operates on the global docker daemon, NOT a compose project: the base
// image is shared across every Frank project on the host. It therefore shells
// out to `docker` directly rather than reusing docker.Client (which always
// wraps `docker compose -f .frank/compose.yaml`).
func EnsureBase(engine *template.Engine, cfg *config.Config) error {
	rendered, err := Render(engine, cfg)
	if err != nil {
		return fmt.Errorf("render base Dockerfile: %w", err)
	}

	hash := Hash(rendered)
	tag := Tag(cfg)

	// oldID from this pre-lock inspect is intentionally discarded: it is
	// re-captured under the lock below, and only the locked value is used for
	// the post-build prune (the pre-lock ID could be stale by then anyway).
	present, gotLabel, _ := inspectBase(tag)
	if !needsBuild(present, gotLabel, hash) {
		output.Detail(fmt.Sprintf("base image %s up to date", tag))
		return nil
	}

	// Serialize concurrent builds of the same tuple behind a host file lock so
	// parallel `frank up` invocations don't race the same tag.
	lockPath, err := baseLockPath(tag)
	if err != nil {
		return err
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open base lock %s: %w", lockPath, err)
	}

	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock base image build: %w", err)
	}

	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	// Re-check under the lock: another holder may have finished building while
	// we waited, so we don't rebuild needlessly.
	var oldID string

	present, gotLabel, oldID = inspectBase(tag)
	if !needsBuild(present, gotLabel, hash) {
		output.Detail(fmt.Sprintf("base image %s up to date", tag))
		return nil
	}

	region := output.Region(fmt.Sprintf("Building base image %s", tag))
	if err := buildBase(tag, hash, rendered, region); err != nil {
		region.Stop(err)
		return fmt.Errorf("build base image %s: %w", tag, err)
	}

	region.Stop(nil)

	// Best-effort prune of the prior base: an in-place rebuild leaves the old
	// layers dangling as <none> (~2GB), so reap it. Ignore errors ("image is
	// in use" etc. are not fatal here).
	if oldID != "" {
		if _, newID, _ := inspectID(tag); newID != "" && newID != oldID {
			pruneImage(oldID)
		}
	}

	return nil
}

// inspectBase inspects tag's frank.base.hash label. present is false when the
// image is absent (inspect exits non-zero). When present, gotLabel is the
// trimmed label value and oldID is the image ID (captured for later prune).
func inspectBase(tag string) (present bool, gotLabel, oldID string) {
	cmd := exec.Command("docker", "image", "inspect", tag,
		"--format", fmt.Sprintf(`{{ index .Config.Labels "%s" }}`, labelKey))

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return false, "", ""
	}

	gotLabel = strings.TrimSpace(stdout.String())
	_, oldID, _ = inspectID(tag)

	return true, gotLabel, oldID
}

// inspectID returns the image ID for tag. present is false when absent.
func inspectID(tag string) (present bool, id string, err error) {
	cmd := exec.Command("docker", "image", "inspect", tag, "--format", "{{.Id}}")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return false, "", err
	}

	return true, strings.TrimSpace(stdout.String()), nil
}

// buildBase builds the base image from rendered, with an empty build context:
// the Dockerfile is fed on stdin (`docker build ... -`). The empty context is
// load-bearing — it keeps the base byte-identical across every project so
// docker maximizes layer dedup. Non-verbose runs discard docker's output.
func buildBase(tag, hash, rendered string, w io.Writer) error {
	cmd := exec.Command("docker", "build",
		"--progress=plain",
		"--label", labelKey+"="+hash,
		"-t", tag,
		"-")
	cmd.Stdin = strings.NewReader(rendered)
	cmd.Stdout = w
	cmd.Stderr = w

	return cmd.Run()
}

// pruneImage removes an image by ID, best-effort (errors ignored).
func pruneImage(id string) {
	cmd := exec.Command("docker", "rmi", id)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	_ = cmd.Run()
}

// baseLockPath returns the host lock-file path for tag, under the user cache
// dir's frank/ subdir, creating that dir as needed. Tag separators (/ :) are
// replaced so the name is filesystem-safe.
func baseLockPath(tag string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}

	dir := filepath.Join(cacheDir, "frank")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create lock dir %s: %w", dir, err)
	}

	return filepath.Join(dir, "base-"+sanitizeTag(tag)+".lock"), nil
}

// sanitizeTag replaces filesystem-unsafe characters in a docker tag with "-".
func sanitizeTag(tag string) string {
	return strings.NewReplacer("/", "-", ":", "-").Replace(tag)
}

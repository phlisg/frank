// Package baseimage derives the identity of Frank's shared base runtime image:
// its tag, the rendered base Dockerfile, and a content hash of that render.
// The tag encodes the (php, runtime, node, pg) tuple so distinct tuples never
// clobber each other; the hash catches changes to the base template body within
// a single tuple. Together they back the frank.base.hash staleness check used by
// ensure-base.
package baseimage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/template"
)

const (
	// NodeVersion and PostgresVersion mirror the base Dockerfile ARG defaults
	// (NODE_VERSION=24, POSTGRES_VERSION=17) and the thin FROM tag suffix
	// (node24-pg17). They are part of the tag, so keep them in lockstep with
	// both templates.
	NodeVersion     = "24"
	PostgresVersion = "17"
)

// Tag returns the base image tag for cfg, e.g.
// "frank/runtime:8.5-frankenphp-node24-pg17". The format is byte-for-byte
// identical to the FROM line in the thin runtime Dockerfile templates.
func Tag(cfg *config.Config) string {
	return fmt.Sprintf(
		"frank/runtime:%s-%s-node%s-pg%s",
		cfg.PHP.Version, cfg.PHP.Runtime, NodeVersion, PostgresVersion,
	)
}

// Render renders the base Dockerfile for cfg's runtime. The base template only
// references PHPVersion, so no other fields are populated.
func Render(engine *template.Engine, cfg *config.Config) (string, error) {
	return engine.RenderRuntime(
		cfg.PHP.Runtime,
		"base.Dockerfile.tmpl",
		template.Data{PHPVersion: cfg.PHP.Version},
	)
}

// Hash returns the sha256 hex digest of the rendered base Dockerfile. This is
// the value of the frank.base.hash image label.
func Hash(rendered string) string {
	sum := sha256.Sum256([]byte(rendered))
	return hex.EncodeToString(sum[:])
}

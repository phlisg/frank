# Changelog

All notable changes to Frank are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This changelog starts at `v1.11.0`. For earlier history, see the
[git log](https://github.com/phlisg/frank/commits) and release tags.

## [Unreleased]

### Added

- `frank up` now detects **any** `frank.yaml` change and auto-regenerates `.frank/`
  before starting containers — previously only a frank version bump or a
  `php.version`/`php.runtime` change triggered regeneration, so edits to
  `workers`, `services`, ports, aliases, or `node` were silently ignored until
  the next manual `frank generate`.
- `.frank/.state` gains a `configHash` field (sha256 of `frank.yaml`) used to
  detect config drift.

### Changed

- Image rebuilds are now scoped: a config change only forces `--build` when it
  alters the rendered Dockerfile (e.g. `php.version`, `php.runtime`). Pure
  compose-level edits (queue lists, service selection, ports) regenerate without
  a slow image rebuild. The decision compares the freshly-rendered Dockerfile to
  the on-disk copy, so it stays correct automatically as Dockerfile inputs grow.

### Fixed

- Editing `workers.queue[].queues` (or any other non-image config) and running
  `frank down && frank up -d` now applies the change. Workers previously kept
  running the stale `--queue` list because compose was never regenerated.

### Notes

- If you hand-edited files under `.frank/`, a subsequent `frank up` that also
  sees a `frank.yaml` change will overwrite them — as it already did on version
  bumps. Edit `frank.yaml`, not the generated files.

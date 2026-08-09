# Changelog

## [1.0.0](https://github.com/edge2992/dw/compare/v0.7.0...v1.0.0) (2026-08-09)


### ⚠ BREAKING CHANGES

* every subcommand and the on-disk layout changed. There is no migration path or compatibility shim; existing ~/dw workspaces are not recognized by the new binary.

### Features

* rebuild dw around STATE.md and a single root CLAUDE.md ([3d6130b](https://github.com/edge2992/dw/commit/3d6130b29d952c8c699d2f1ae2e093a64459b410))


### Bug Fixes

* normalize DW_ROOT and stop capping STATE.md scans ([d1b83eb](https://github.com/edge2992/dw/commit/d1b83eb2537ab20c2e0d0f93ad7c2aec75d6fe8d))

## [0.7.0](https://github.com/edge2992/dw/compare/v0.6.0...v0.7.0) (2026-08-03)


### Features

* carry session context across workspace visits ([5262e35](https://github.com/edge2992/dw/commit/5262e3593eaa3b7b75dfb4ffd1f92bb7f5ae2009))
* carry session context across workspace visits ([2dc5b39](https://github.com/edge2992/dw/commit/2dc5b3934c54538b7bde6a182672941e5f5af8cb))
* scaffold a per-category CLAUDE.md ([4b22ec6](https://github.com/edge2992/dw/commit/4b22ec6fe097e06037e9f4eeda879f78a6e51eb2))
* scaffold a per-category CLAUDE.md ([1dde7b3](https://github.com/edge2992/dw/commit/1dde7b3ff19f670c5ee9cb83c533e7d654a3a2a7))


### Bug Fixes

* create scaffold files atomically ([41f1313](https://github.com/edge2992/dw/commit/41f13139e328ee60b3bb84db3d9cb659012e7ae1))

## [0.6.0](https://github.com/edge2992/dw/compare/v0.5.0...v0.6.0) (2026-06-27)


### ⚠ BREAKING CHANGES

* DW_ROOT is no longer read; set `root:` in ~/.config/dw/config.yml instead. Templates moved from ~/.config/discussion/templates/ to ~/.config/dw/templates/.

### Features

* drive dw via ~/.config/dw/config.yml ([#19](https://github.com/edge2992/dw/issues/19)) ([42447be](https://github.com/edge2992/dw/commit/42447bee5c691d2d47d8f22d58eea4664410a75a))

## [0.5.0](https://github.com/edge2992/dw/compare/v0.4.0...v0.5.0) (2026-06-20)


### Features

* add dw new and dw init subcommands ([#17](https://github.com/edge2992/dw/issues/17)) ([d31d27a](https://github.com/edge2992/dw/commit/d31d27a197a478160b0df782dd24b77527303203))

## [0.4.0](https://github.com/edge2992/dw/compare/v0.3.0...v0.4.0) (2026-06-17)


### Features

* switch dw runtime flow to English ([#14](https://github.com/edge2992/dw/issues/14)) ([79b687c](https://github.com/edge2992/dw/commit/79b687caa627d38c23f5feb822e279c1313bf1ba))

## [0.3.0](https://github.com/edge2992/dw/compare/v0.2.0...v0.3.0) (2026-06-17)


### ⚠ BREAKING CHANGES

* default root ~/Discussion -> ~/dw; DISCUSSION_ROOT -> DW_ROOT.

### Features

* ghq-style subcommands, move root to ~/dw ([#10](https://github.com/edge2992/dw/issues/10)) ([387666e](https://github.com/edge2992/dw/commit/387666ee4f03f3b0c68fb61aeb51b1335c77c192))

## [0.2.0](https://github.com/edge2992/dw/compare/v0.1.0...v0.2.0) (2026-06-17)


### Features

* convention-based per-category templates ([#8](https://github.com/edge2992/dw/issues/8)) ([0ebec38](https://github.com/edge2992/dw/commit/0ebec38aa98c67b5ec8c686916ee4f92900f54c0))

## 0.1.0 (2026-06-17)


### Features

* add dw interactive discussion workspace picker ([c509a15](https://github.com/edge2992/dw/commit/c509a155b65a4506bb95788d90c6c4c3265e327b))
* keep typed topic as README title and mark the pinned project ([#2](https://github.com/edge2992/dw/issues/2)) ([918e339](https://github.com/edge2992/dw/commit/918e339221283ebf496bec70110c7418993d93ae))
* pin last workspace, inline meta, and dated-first ordering ([#1](https://github.com/edge2992/dw/issues/1)) ([adb4414](https://github.com/edge2992/dw/commit/adb44145acd34e39b74604de61906a6e876bae96))


### Bug Fixes

* drop component prefix from release tags and reset to v0.1.0 ([#5](https://github.com/edge2992/dw/issues/5)) ([0b34236](https://github.com/edge2992/dw/commit/0b3423642cf35200c274f3e60a636f9e9317f4e5))
* unicode-safe slugify, coverage badge, review fixes ([2bac91f](https://github.com/edge2992/dw/commit/2bac91f8f2d97af907bb6fb91f9a876127787535))

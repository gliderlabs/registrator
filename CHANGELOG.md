# Change Log
All notable changes to this project will be documented in this file.

## [Unreleased][unreleased]
### Fixed

### Added

### Removed

### Changed


## [v5] - 2015-02-18
### Added
- Automated, PR-driven release process
- Development Dockerfile and make task
- CircleCI config with artifacts for every build
- `--version` flag to see version

### Changed
- Base container is now Alpine
- Built entirely in Docker
- Moved to gliderlabs organization
- New versioning scheme
- Release artifact now saved container image

### Removed
- Dropped unnecessary layers in Dockerfile
- Dropped Godeps for now


[unreleased]: https://github.com/gliderlabs/registrator/compare/v5...HEAD
[v5]: https://github.com/gliderlabs/registrator/compare/v0.4.0...v5

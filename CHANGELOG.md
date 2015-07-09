# Change Log
All notable changes to this project will be documented in this file.

## [Unreleased][unreleased]
### Fixed
- Support for etcd v0 and v2
- Panic from invalid skydns2 URI.

### Added
- Optional periodic resyncing of services from containers
- More error logging for registries
- Support for services on containers with `--net=host`
- Added `extensions.go` file for adding/disabling components
- Interpolate SERVICE_PORT and SERVICE_IP in SERVICE_X_CHECK_SCRIPT
- Ability to force IP for a service in consul
- Implemented ping for every service registry
- Option to only deregister containers cleanly shutdown #113

### Removed

### Changed
- Overall refactoring and cleanup
- Decoupled registries into subpackages using extpoints
- Add full TTL support for Consul 0.5.0.
- Support multiple Consul checks per service (such as a TTL and script)


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

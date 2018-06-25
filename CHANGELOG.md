# Change Log
All notable changes to this project will be documented in this file.

## [Unreleased][unreleased]
### Fixed

### Added
- Populated Consul Meta for the service using service.Attrs

### Removed

### Changed

## [v8] - 2018-06-25
### Fixed
- Remove stale/dangling services from registrator #339
- Fix missing ServiceAddress (i.e. HostIP) in overlay networks #345
- Fix specific port names not overriding port suffix #245
- Fix multiple ports named service #194
- Synchornize etcd cluster in registrator on service registration #221
- Fix missing protocol when creating containers via Docker API #227

### Added
- Populated Consul Meta for the service using service.Attrs #620
- CircleCI 2.0 support
- Add windows runtime detection #571
- Support to use exposed ports when in host networking mode #268
- Consul initial check status #445
- Support for Consul HTTPS health checks #338
- Support for Consul TCP health checks #357
- TLS support for Consul backend #394
- Support for Consul unix sockets #336
- Support for Docker multi host networking #303
- Retries to backend at startup #235

### Removed

### Changed
- Multistage Docke build with alpine:3.7 as base image


## [v7] - 2016-03-05
### Fixed
- Providing a SERVICE_NAME for a container with multiple ports exposed would cause services to overwrite each other
- dd3ab2e Fix specific port names not overriding port suffix

### Added
- bridge.Ping - calls adapter.Ping
- Consul TCP Health Check
- Support for Consul unix sockets
- Basic Zookeper backend
- Support for Docker multi host networking
- Default to tcp for PortType if not provided
- Sync etcd cluster on service registration
- Support hostip for overlay network
- Cleanup dangling services
- Startup backend service connection retry

### Removed

### Changed
- Upgraded base image to alpine:3.2 and go 1.4
- bridge.New returns an error instead of calling log.Fatal
- bridge.New will not attempt to ping an adapter.
- Specifying a SERVICE_NAME for containers exposing multiple ports will now result in a named service per port. #194
- Etcd uses port 2379 instead of 4001 #340
- Setup Docker client from environment
- Use exit status to determine if container was killed

## [v6] - 2015-08-07
### Fixed
- Support for etcd v0 and v2
- Panic from invalid skydns2 URI.

### Added
- Basic zookeeper adapter
- Optional periodic resyncing of services from containers
- More error logging for registries
- Support for services on containers with `--net=host`
- Added `extensions.go` file for adding/disabling components
- Interpolate SERVICE_PORT and SERVICE_IP in SERVICE_X_CHECK_SCRIPT
- Ability to force IP for a service in Consul
- Implemented initial ping for every service registry
- Option to only deregister containers cleanly shutdown #113
- Added support for label metadata along with environment variables

### Removed

### Changed
- Overall refactoring and cleanup
- Decoupled registries into subpackages using extpoints
- Replaced check-http script with Consul's native HTTP checks


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


[unreleased]: https://github.com/gliderlabs/registrator/compare/v7...HEAD
[v7]: https://github.com/gliderlabs/registrator/compare/v6...v7
[v6]: https://github.com/gliderlabs/registrator/compare/v5...v6
[v5]: https://github.com/gliderlabs/registrator/compare/v0.4.0...v5

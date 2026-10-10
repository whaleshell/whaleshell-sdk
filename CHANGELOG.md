# Changelog

## [Unreleased]

## [v0.1.6] - 2026-10-11

### Changed

- Rename the module, runtime identifiers and project references to the `cautem` namespace.

## [v0.1.0-beta.1] - 2026-10-10

### Added

- Provide typed Go clients for Control RPC gateway information, inference, sandbox and log management, services, settings and secrets.
- Carry request IDs, resource versions and operation recovery through the management client.

## [v0.1.0-alpha.2] - 2026-10-07

### Added

- Scoped provider profile catalog operations and refresh metadata in the public client API.
- Match OpenShell gateway request/response payloads for workspace services and SSH relay operations.
- Encode URL path segments and decode response bodies with operation context instead of discarding transport details.

### Changed

- Distribute the module under Apache-2.0 and link the public gateway/API documentation from the README.

## [v0.0.2-alpha.1] - 2026-09-28

### Added

- `Proposal.SecurityFlagged` for gateway approval flows that gate bulk approve.

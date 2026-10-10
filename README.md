<h1 align="center">cautem-sdk</h1>

<p align="center">
  <strong>Go client for cautem-gateway</strong><br>
  generated gRPC resources and the versioned cautem control API.
</p>
<p align="center">
  <a href="https://github.com/cautem/cautem-sdk/actions/workflows/ci.yml"><img src="https://github.com/cautem/cautem-sdk/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/cautem/cautem-sdk"><img src="https://pkg.go.dev/badge/github.com/cautem/cautem-sdk.svg" alt="Go Reference"></a>
  <a href="https://www.apache.org/licenses/LICENSE-2.0"><img src="https://img.shields.io/badge/License-Apache--2.0-blue.svg" alt="License"></a>
  <a href="https://github.com/cautem/cautem-sdk"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go Version"></a>
</p>
<p align="center">
  <sub>Part of the <a href="https://github.com/cautem">cautem / cautem</a> ecosystem</sub>
</p>

---

## Overview

The [gateway guide](https://cautem.github.io/sandbox.dev/guides/gateway/) describes the service this client calls. [OpenShell compatibility](https://cautem.github.io/sandbox.dev/reference/openshell-compatibility/) is tracked separately from this SDK's HTTP API.

**cautem-sdk** talks to [cautem-gateway](https://github.com/cautem/cautem-gateway) through typed RPC clients. Sandbox inventory, detail, lifecycle, logs, provider profiles, partial provider credential updates, policy workflows, and service/template listings use generated native gRPC clients for `cautem.control.v1`; command execution, provider instances/attachments/refresh, workspace resources, and SSH session issue/revoke use the pinned OpenShell Go SDK. Settings, service/template details and writes, supervisor registration, and some other cautem-only workflows still use REST routes and are being migrated. Interactive byte streams stay on the CLI (`cautem connect`). REST compatibility is not a migration requirement.

### Key Features

| Category | Capabilities |
|----------|--------------|
| **Client** | `go/cautem` — resource facade over generated RPC clients; some legacy HTTP methods remain during migration |
| **Execution** | OpenShell `ExecSandbox` RPC; interactive TTY stays on the CLI |
| **Proposals** | List / get / approve / reject |

---

## Installation

For source development, use the sibling `go.work` workspace and run `go test ./...` here. The control client depends on the gateway's generated Go contract package. Publish the gateway beta containing that package before publishing the matching SDK beta.

**Requirements:** Go 1.27+

---

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/cautem/cautem-sdk/go/cautem"
)

func main() {
    c := cautem.New("http://127.0.0.1:7443")
    list, err := c.ListControlSandboxes(context.Background(), "default", false)
    if err != nil {
        panic(err)
    }
    fmt.Println(list)
}
```

---

## Package Structure

| Path | Purpose |
|------|---------|
| `go/cautem` | Public Go SDK |
| `internal/gatewayclient/` | HTTP API implementation (not importable outside the module) |

`ListControlSandboxes(ctx, workspace, allWorkspaces)`, `GetControlSandbox`,
`GetControlSandboxLogs`, and `WatchControlSandboxLogs` use
`cautem.control.v1` over native gRPC. `Whoami(ctx)` uses `GetViewer`. `List(ctx)` uses the control API across all workspaces
visible to the caller, and `Get(ctx, name)` selects the default workspace. The
`Exec(ctx, name, argv...)` method uses the pinned OpenShell Go SDK. The CLI uses native gRPC for `sandbox list/get`, filtered log snapshots, and a single
sandbox's live log stream. `logs --all` enumerates caller-visible workspaces and
uses bounded concurrent gRPC streams (up to 24 sandboxes per invocation).


---

## Related

| Resource | Link |
|----------|------|
| Roadmap | [ROADMAP.md](./ROADMAP.md) |
| Organization | [https://github.com/cautem](https://github.com/cautem) |
| Organization overview | [github.com/cautem](https://github.com/cautem) |
| pkg.go.dev | [`github.com/cautem/cautem-sdk`](https://pkg.go.dev/github.com/cautem/cautem-sdk) |

## License

[Apache-2.0](./LICENSE) © cautem

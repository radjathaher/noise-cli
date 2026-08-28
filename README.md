# noise-cli

`noise` is a machine-first CLI for Noise reporting and management. It reads
public reporting data through REST. It uses Noise MCP tools for detailed reads
and management actions.

Documentation:

- [REST, CLI, and MCP capability matrix](docs/noise-capability-matrix.md)
- [Codex, Claude, and Hermes MCP + skill setup](docs/mcp-and-skill-setup.md)
- [Noise campaign operator skill](skills/noise-campaign-operator/SKILL.md)

```text
                         ┌─────────────────────────────┐
                         │            noise            │
                         │ JSON stdout · stderr errors │
                         └──────────────┬──────────────┘
                              read     │     manage
                    ┌──────────────────┴──────────────────┐
                    ▼                                     ▼
       ┌────────────────────────┐           ┌────────────────────────┐
       │ Reporting REST API     │           │ Noise MCP              │
       │ health, reports, posts │           │ detail, writes, audit  │
       │ campaign/playbook list │           │ tool discovery/calls   │
       └────────────────────────┘           └────────────────────────┘
```

## Install

Release archives support these targets:

| System | Architecture | Target |
| --- | --- | --- |
| macOS | Apple Silicon | `darwin-aarch64` |
| macOS | Intel | `darwin-x86_64` |
| Linux | ARM64 | `linux-aarch64` |
| Linux | AMD64 | `linux-x86_64` |

Download the archive and `SHA256SUMS` from the GitHub release. Then verify and
install the binary:

```sh
version=0.1.0
target=darwin-aarch64
archive="noise-cli-${version}-${target}.tar.gz"
base="https://github.com/radjathaher/noise-cli/releases/download/v${version}"

curl -fLO "${base}/${archive}"
curl -fLO "${base}/SHA256SUMS"
grep " ${archive}$" SHA256SUMS > SHA256SUMS.one

# macOS
shasum -a 256 -c SHA256SUMS.one

# Linux
sha256sum -c SHA256SUMS.one

tar -xzf "$archive"
install -m 0755 noise "$HOME/.local/bin/noise"
noise --version
```

Use a directory that is in `PATH`. Release archives contain one `noise` binary
at the archive root.

## Authentication and environment

REST reads need a Noise organization and API key:

```sh
export NOISE_ORG="<organization>"
export NOISE_API_KEY="<api-key>"
```

MCP commands can use a fixed access token:

```sh
export NOISE_MCP_ACCESS_TOKEN="<access-token>"
```

MCP commands can also use OAuth client and refresh credentials:

```sh
export NOISE_MCP_CLIENT_ID="<client-id>"
export NOISE_MCP_CLIENT_SECRET="<client-secret>" # Set only when required.
export NOISE_MCP_REFRESH_TOKEN="<refresh-token>"
```

Environment credentials take priority over a saved interactive login. An
interactive login requires `NOISE_MCP_CLIENT_ID`. The CLI does not implement
dynamic client registration. With no client secret, login uses PKCE:

```sh
noise auth login
noise auth status
noise auth logout
```

If `NOISE_MCP_CLIENT_SECRET` is also set, `auth login` uses the OAuth client
credentials flow instead of PKCE.

`auth logout` removes the saved local session. It does not rotate REST keys.
Use the Noise control plane to revoke a server-side token when revocation is
required.

Configuration:

| Name | Purpose | Required |
| --- | --- | --- |
| `NOISE_ORG` | REST organization header | REST commands except `health` |
| `NOISE_API_KEY` | REST API token | REST commands except `health` |
| `NOISE_MCP_ACCESS_TOKEN` | Direct MCP bearer token | One MCP auth method |
| `NOISE_MCP_CLIENT_ID` | OAuth client ID | OAuth client flow only |
| `NOISE_MCP_CLIENT_SECRET` | OAuth client secret | Only for a confidential client |
| `NOISE_MCP_REFRESH_TOKEN` | OAuth refresh token | Refresh flow only |
| `NOISE_BASE_URL` | Override the Reporting API URL | No |
| `NOISE_MCP_URL` | Override the MCP endpoint URL | No |

Do not put credentials in flags, command history, source files, or logs. Load
them from the process environment or a secret manager. Use HTTPS endpoints in
production.

## Output contract

- Successful commands write JSON to stdout.
- Errors and debug logs write to stderr.
- `--pretty` adds JSON indentation. It does not change fields.
- `--debug` writes redacted diagnostics to stderr.
- `--timeout 30s` sets the request deadline.
- `--base-url` and `--mcp-url` override the default endpoints.

This separation is safe for pipes:

```sh
noise campaign list > campaigns.json
noise --pretty campaign list | jq '.data'
```

## Command map

| Command | Transport | Action |
| --- | --- | --- |
| `noise health` | REST | Check service health; no auth required |
| `noise report daily` | REST | Read daily metrics |
| `noise post list` | REST | Read posts |
| `noise campaign list` | REST | List campaigns |
| `noise playbook list` | REST | List playbooks |
| `noise auth login\|status\|logout` | OAuth/local | Manage MCP login state |
| `noise mcp tools` | MCP | List available tools |
| `noise mcp describe` | MCP | Read one tool schema |
| `noise mcp call` | MCP | Read, preview, or apply a discovered tool |

The CLI does not assume typed Noise MCP schemas. Campaign detail and management,
playbook detail and management, and audit reads are available through guarded
`noise mcp call` only when the connected server exposes the required tools.
Use discovery before automation:

```sh
noise mcp tools --pretty
noise mcp describe <tool-name> --pretty
noise mcp call --help
```

## Reporting examples

Read daily metrics:

```sh
noise report daily \
  --start-date 2026-08-01 \
  --end-date 2026-08-07 \
  --group-by campaign,playbook \
  --metrics spend,views,posts,creators,cpm,estimated_installs \
  --pretty
```

Filter daily metrics by campaign or playbook:

```sh
noise report daily \
  --start-date 2026-08-01 \
  --end-date 2026-08-07 \
  --campaign-id 123

noise report daily \
  --start-date 2026-08-01 \
  --end-date 2026-08-07 \
  --playbook-id 0
```

Playbook ID `0` means Custom Posts. Date ranges use inclusive UTC dates.

List posts:

```sh
noise post list \
  --start-date 2026-08-01 \
  --end-date 2026-08-07 \
  --mode created \
  --campaign-id 123,456 \
  --page 1 \
  --page-size 25 \
  --sort views \
  --order desc
```

Run `noise <command> --help` for the accepted fields and limits.

## Preview and apply safety

`noise mcp call` executes a tool directly only when MCP annotations prove that
the tool is read-only. For other tools, the CLI requires a Boolean `confirm`
input. The CLI sends `confirm=false` for the preview. Add `--apply` only after
review.

```text
┌───────────────┐       ┌────────────────┐       ┌─────────────────┐
│ requested edit│──────►│ server preview │──────►│ JSON plan stdout│
└───────────────┘       └────────────────┘       └────────┬────────┘
                                                         │ review
                        ┌────────────────┐       ┌────────▼────────┐
                        │ manual verify  │◄──────│ repeat --apply │
                        └────────────────┘       └─────────────────┘
```

An apply run creates the server preview first. If the preview succeeds, the CLI
sends `confirm=true`. The result contains both the preview and apply response.
The CLI does not retry a management write automatically. After apply, use a
discovered detail or audit tool for verification.

If the CLI cannot classify a tool call as read-only or mutating, it stops with
exit code `9`. Do not retry an uncertain write. Check current server state and
the audit log first.

## Exit codes

| Code | Meaning |
| ---: | --- |
| `0` | Success |
| `2` | Invalid command, flag, input, or local configuration |
| `3` | Missing, expired, or rejected credentials |
| `4` | Resource or required MCP capability not found |
| `5` | State or version conflict |
| `6` | Noise validation rejected the request |
| `7` | Rate limit |
| `8` | Network failure or upstream server failure |
| `9` | Write result is ambiguous; inspect state before retry |

## Headless use

Use environment credentials for CI, cron, and agents. Do not use the
interactive PKCE flow in a headless job.

```sh
set -euo pipefail

: "${NOISE_ORG:?missing NOISE_ORG}"
: "${NOISE_API_KEY:?missing NOISE_API_KEY}"
: "${NOISE_MCP_ACCESS_TOKEN:?missing NOISE_MCP_ACCESS_TOKEN}"

noise health >/tmp/noise-health.json
noise mcp tools >/tmp/noise-tools.json
noise report daily \
  --start-date 2026-08-01 \
  --end-date 2026-08-01 \
  > /tmp/noise-daily.json
```

For a long-running job, use a refresh token and client credentials instead of
a short-lived fixed access token. Restrict secret access to the job identity.

## Token rotation

REST key rotation:

1. Create a new Noise API key.
2. Update `NOISE_API_KEY` in the secret manager.
3. Run `noise campaign list` with the new job environment.
4. Revoke the old key after verification.

MCP token rotation:

1. Create a new access token or refresh-token set.
2. Update the MCP environment variables in the secret manager.
3. Run `noise auth status` and `noise mcp tools`.
4. Revoke the old token after verification.

For an interactive session, run `noise auth logout`, then `noise auth login`.
Never print a token to verify it.

## Rollback

Noise is the source of truth. The CLI has no general transaction or automatic
rollback.

Before a management change:

1. Read and save the current resource JSON.
2. Run `noise mcp call <tool> --arguments '<json>'` without `--apply`.
3. Review the preview and resource identifier.
4. Repeat the same command with `--apply`.
5. Read the resource and audit records.

To roll back, preview and apply a discovered tool that restores the saved
values. Some server actions can be irreversible. Run `noise mcp describe` and
check the schema before apply.

## Agent runbook

```text
┌──────────────┐   ┌──────────────┐   ┌────────────────┐
│ auth status  │   │ health       │   │ mcp tools      │
│ credentials  │   │ REST reach   │   │ capabilities   │
└──────┬───────┘   └──────┬───────┘   └───────┬────────┘
       └──────────────────┼────────────────────┘
                          ▼
                 ┌─────────────────┐
                 │ read state      │
                 │ save JSON + IDs │
                 └────────┬────────┘
                   ┌──────┴──────┐
                   ▼             ▼
            ┌────────────┐  ┌────────────┐
            │ preview    │  │ stop       │
            │ no --apply │  │ on drift   │
            └──────┬─────┘  └────────────┘
                   ▼
            ┌────────────┐
            │ apply once │
            └──────┬─────┘
                   ▼
          ┌──────────────────┐
          │ readback + audit │
          └──────────────────┘
```

Rules for agents:

- Discover MCP tools at the start of each run.
- Keep stdout as the machine-readable result channel.
- Keep identifiers from the read response. Do not guess names or IDs.
- Treat a changed preview as drift. Stop and read state again.
- Apply one logical change at a time.
- On exit `9`, inspect resource state and audit records before any retry.
- Store before-and-after JSON with the job record.

## Limitations

- REST support is read-only and covers the public Reporting API surface listed
  above.
- Detailed reads and management depend on the tools exposed by Noise MCP.
- MCP tool names and schemas can change independently of this CLI release.
- Typed campaign, playbook, and audit MCP wrappers are deferred until an
  authenticated `tools/list` response can define and verify exact schemas.
- Readback and audit verification are operator steps. They work only when the
  server exposes the needed tools.
- The CLI cannot make multiple MCP writes atomic.
- The CLI does not provide automatic rollback.
- Pagination remains explicit. Scripts must request and combine pages.

## Releases

Tags matching semantic versions such as `v0.1.0` run formatting, vet, race
tests, and builds before release. Each release contains:

```text
noise-cli-<version>-darwin-aarch64.tar.gz
noise-cli-<version>-darwin-x86_64.tar.gz
noise-cli-<version>-linux-aarch64.tar.gz
noise-cli-<version>-linux-x86_64.tar.gz
SHA256SUMS
```

Release builds disable CGO, remove local build paths, inject the tag version,
and put the `noise` binary at the archive root.

## License

MIT. See `LICENSE`.

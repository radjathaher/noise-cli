# Noise REST, CLI, and MCP capability matrix

Verified on 2026-08-28.

## Finding

Noise has two service interfaces. The `noise` CLI can use both.

```text
                       ┌──────────────────────────────┐
                       │ noise CLI                    │
                       │ one shell/JSON interface     │
                       └──────────────┬───────────────┘
                        typed REST    │    generic MCP
                ┌─────────────────────┴─────────────────────┐
                ▼                                           ▼
     ┌────────────────────────┐                ┌────────────────────────┐
     │ Reporting REST API     │                │ Noise MCP              │
     │ stable read endpoints  │                │ reads + lifecycle      │
     └────────────────────────┘                └────────────────────────┘
```

Set relation:

```text
Reporting business reads:  REST ⊂ MCP
All HTTP operations:       neither is a strict subset
                           REST alone has /api/health
Full CLI reachability:     typed REST ∪ guarded generic MCP
```

The important correction: the CLI is not only a wrapper for the Reporting REST
API. Its typed commands cover REST. Its `noise mcp` commands discover and call
the MCP server dynamically.

MCP is a management superset, but it is not a complete CRUD lifecycle for every
Noise object. In the observed tool set, an agent can create a playbook but
cannot create a campaign.

## Capability-by-capability

Legend:

- **Typed**: a dedicated CLI command with local flags.
- **Generic**: available through `noise mcp call` after live discovery.
- **—**: no observed operation on that interface.

| Capability | Raw REST | Typed CLI | Noise MCP | Generic CLI | Notes |
| --- | --- | --- | --- | --- | --- |
| Service health | `GET /api/health` | `noise health` | — | — | REST-only transport check; no auth |
| Daily reporting | `GET /v1/reports/daily` | `noise report daily` | `get_daily_report` | Yes | Noise documents reporting-data parity; argument parity is not proven |
| List/report posts | `GET /v1/posts` | `noise post list` | `list_posts` | Yes | REST has date, mode, ID, page, and sort filters |
| List campaigns | `GET /v1/campaigns` | `noise campaign list` | `list_campaigns` | Yes | Reporting read |
| Campaign detail | — | — | `get_campaign_details` | Yes | MCP-only detailed read |
| Campaign status | — | — | `set_campaign_status` | Yes | Pause or activate; preview and confirmation required |
| Campaign budget | — | — | `update_campaign_budget` | Yes | Spend-affecting write |
| Campaign daily target | — | — | `set_daily_target` | Yes | Management write |
| Campaign rates | — | — | `update_campaign_rates` | Yes | Management write |
| List playbooks | `GET /v1/playbooks` | `noise playbook list` | `list_playbooks` | Yes | Reporting read |
| Playbook detail and slides | — | — | `get_playbook_details` | Yes | MCP-only detailed read |
| Create playbook | — | — | `create_playbook` | Yes | MCP-only lifecycle write |
| Edit playbook | — | — | `update_playbook` | Yes | Content and metadata write |
| Save playbook | — | — | `save_playbook` | Yes | Separate save operation observed in the live tool list |
| Playbook status | — | — | `set_playbook_status` | Yes | Pause or activate |
| Generate slide image | — | — | `generate_slide_image` | Yes | Image generation for a playbook slide |
| Change audit log | — | — | `get_audit_log` | Yes | Readback and attribution verification |

## Requested lifecycle gaps

| Requested operation | REST | MCP | Result |
| --- | --- | --- | --- |
| Create campaign | — | — | Not exposed |
| Delete or archive campaign | — | — | Not exposed as a distinct tool |
| Create or edit posts | — | — | Posts are reporting reads only |
| Delete playbook | — | — | Not exposed as a distinct tool |
| Manage creators | — | — | Not exposed in the observed tool list |
| Manage a `card` object | — | — | No such observed domain object; use a playbook slide when that is the user's meaning |
| Manage a `storyboard` object | — | — | No such observed domain object; playbook content can represent the plan |

Therefore, “full lifecycle” is accurate for playbook authoring only in a broad
sense: create, edit, save, change status, and generate slide images. It is not
strict CRUD because no delete tool was observed. Campaign MCP support manages
existing campaigns; it does not create them.

## Raw REST surface

The published OpenAPI document exposes five operations:

| Method and path | Authentication | CLI implementation |
| --- | --- | --- |
| `GET /api/health` | None | `noise health` |
| `GET /v1/reports/daily` | `x-noise-org`, `x-noise-token` | `noise report daily` |
| `GET /v1/posts` | `x-noise-org`, `x-noise-token` | `noise post list` |
| `GET /v1/campaigns` | `x-noise-org`, `x-noise-token` | `noise campaign list` |
| `GET /v1/playbooks` | `x-noise-org`, `x-noise-token` | `noise playbook list` |

There are no published REST create, update, status, budget, rate, target, or
playbook-authoring endpoints.

Code evidence:

- `internal/reporting/client.go:98-124` maps the five REST paths.
- `cmd/reporting_commands.go` exposes all five as typed commands.
- `internal/reporting/params.go:11-191` validates the report and post filters.

## MCP surface

An authenticated Codex MCP connection exposed 16 tools on 2026-08-28:

```text
Reads                         Campaign writes
──────────────────────────    ──────────────────────────
get_daily_report              set_campaign_status
list_posts                    update_campaign_budget
list_campaigns                set_daily_target
get_campaign_details          update_campaign_rates
list_playbooks
get_playbook_details          Playbook writes
get_audit_log                 ──────────────────────────
                              create_playbook
                              update_playbook
                              save_playbook
                              set_playbook_status
                              generate_slide_image
```

This is a live server inventory, not a versioned contract in this repository.
Agents must discover tools and inspect their input schemas at run time.

## CLI MCP extension

| CLI command | Purpose |
| --- | --- |
| `noise auth login\|status\|logout` | Manage the CLI's MCP OAuth state |
| `noise mcp tools` | Call MCP `tools/list` |
| `noise mcp describe <name>` | Return one live tool definition and input schema |
| `noise mcp call <name>` | Run a proven read-only tool or preview a guarded write |
| `noise mcp call <name> --apply` | Preview, then apply a guarded write |

`cmd/management_commands.go:102-186` implements discovery, schema inspection,
read calls, and the write guard. For a write-like tool, the CLI requires a
Boolean `confirm` field. It calls `confirm=false` first and only calls
`confirm=true` with `--apply`.

This makes current MCP business tools reachable through the CLI. It does not
make them typed CLI commands. The agent must still provide schema-valid JSON.

## Parity limits

Noise states that MCP and the Reporting API expose the same reporting data.
This supports data parity for daily metrics, posts, campaigns, and playbooks.

It does not prove exact request-shape parity. For example, the REST OpenAPI
defines its own date, grouping, filter, page, and sort parameters. The live MCP
tools can use different inputs while returning the same underlying data.

Also, MCP can expose protocol resources, prompts, and server instructions in
addition to tools. This review verified the live tool list only. It does not
claim that every possible MCP protocol feature has a CLI wrapper.

## Which interface to use

| Need | Preferred interface | Why |
| --- | --- | --- |
| Agent conversation and lifecycle management | Direct MCP | Native tool schemas, descriptions, and OAuth identity |
| Campaign or playbook write | Direct MCP | Best preview and confirmation interaction |
| Scheduled reporting or shell pipeline | Typed REST CLI | Stable flags and JSON stdout |
| Agent without remote MCP support | Generic CLI MCP | Reaches discovered MCP tools from the shell |
| Reachability probe | REST health | Dedicated unauthenticated endpoint |

## Sources

- [Noise: Connect an AI Client](https://platform.getnoise.com/docs/connect-ai-client)
- [Noise: Reporting API](https://platform.getnoise.com/docs/reporting-api)
- [Noise Reporting OpenAPI](https://reporting.getnoise.com/openapi.yaml)
- [Codex MCP documentation](https://developers.openai.com/codex/mcp/)
- [Hermes MCP documentation](https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp)

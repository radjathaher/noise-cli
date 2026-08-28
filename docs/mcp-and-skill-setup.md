# Noise MCP and operator skill setup

Use the same two-layer pattern as ScreensDesign:

```text
┌─────────────────────────────────────────────────────────────┐
│ Agent client                                                │
│ Codex · Claude · Hermes                                     │
└──────────────────────┬──────────────────────────────────────┘
             behavior │                 live operations
       ┌──────────────┴─────────────┐  ┌──────────────────────┐
       ▼                            │  ▼                      │
┌────────────────────┐             │  ┌──────────────────────┐
│ operator SKILL.md  │             └─►│ Noise remote MCP     │
│ process + safety   │                │ OAuth + tool schemas │
└────────────────────┘                └──────────────────────┘
```

The MCP server supplies capabilities and live data. The skill supplies agent
behavior: interface selection, object mapping, preview, approval, readback, and
audit verification. A skill does not add server capabilities.

## Server

```text
https://reporting.getnoise.com/mcp
```

Noise uses OAuth. Sign in with the email used for the Noise portal. The linked
client sees only explicitly linked brands.

## Codex

Standard Codex CLI:

```sh
codex mcp add noise --url https://reporting.getnoise.com/mcp
codex mcp login noise
codex mcp list
```

On the Cakrawala fleet, the interactive launcher is `cdx`. Use the matching
`cdx mcp` subcommands for MCP configuration. In the TUI, `/mcp` shows the
connected server and tools.

Install the skill for a user:

```sh
mkdir -p ~/.codex/skills
cp -R skills/noise-campaign-operator ~/.codex/skills/
```

Restart Codex after installation. Then ask:

```text
Use $noise-campaign-operator to compare this week's campaigns and preview a
budget change for the weakest campaign. Do not apply it.
```

## Claude

1. Open **Settings → Connectors**.
2. Add a custom connector.
3. Paste `https://reporting.getnoise.com/mcp`.
4. Complete Noise OAuth.

For Claude Code skill discovery:

```sh
mkdir -p ~/.claude/skills
cp -R skills/noise-campaign-operator ~/.claude/skills/
```

Restart Claude Code after installation.

## Hermes

Add this to `~/.hermes/config.yaml`:

```yaml
mcp_servers:
  noise:
    url: "https://reporting.getnoise.com/mcp"
    auth: oauth
```

Then authenticate and verify:

```sh
hermes mcp login noise
hermes mcp list
```

Hermes performs OAuth discovery, client registration, PKCE, refresh, and tool
registration for remote OAuth MCP servers. It registers Noise tools with the
`mcp_noise_` prefix internally.

Install `skills/noise-campaign-operator` in the Agent Skills directory used by
the Hermes installation. Keep one canonical copy from this repository and
deploy it with the machine configuration. Restart the Hermes gateway after the
skill or MCP configuration changes.

## CLI fallback

The standalone CLI login is separate from Codex, Claude, and Hermes OAuth
sessions. It needs one supported CLI credential source:

```text
NOISE_MCP_ACCESS_TOKEN
or
NOISE_MCP_CLIENT_ID + login/refresh credentials
```

The CLI does not implement Dynamic Client Registration or Client ID Metadata
Documents. A successful client OAuth login does not automatically authenticate
`noise mcp tools`.

Verify CLI access:

```sh
noise auth status
noise mcp tools
noise mcp describe get_campaign_details
```

## Safe acceptance test

Do not make a live change during setup.

```text
1. List tools.
2. List campaigns.
3. Read one campaign by its returned ID.
4. Read the audit log.
5. Ask for a budget-change preview only.
6. Confirm that no apply call occurred.
```

Expected lifecycle coverage after authentication:

```text
Campaign                         Playbook
├── list + detail                ├── list + detail
├── status                       ├── create + edit + save
├── budget                       ├── status
├── daily target                 └── slides + images
├── rates
└── reports/posts/audit
```

Important limit: the observed MCP has no campaign-create, campaign-delete,
playbook-delete, post-write, creator-management, card, or storyboard tool. It
manages existing campaigns and provides broad playbook authoring. It is not a
complete CRUD API for every Noise object.

See [the capability matrix](noise-capability-matrix.md) for the verified
REST-versus-MCP comparison.

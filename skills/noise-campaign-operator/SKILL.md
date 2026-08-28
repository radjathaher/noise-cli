---
name: noise-campaign-operator
description: Inspect, report on, create, or change Noise campaigns and playbooks through the Noise MCP server or noise CLI. Use for Noise performance, posts, budgets, rates, daily targets, playbook slides, images, status, and audit-log work.
---

# Noise Campaign Operator

Use Noise MCP as the primary interface for interactive agent work. Use the
`noise` CLI for shell pipelines, scheduled reporting, or when the client cannot
connect to remote MCP.

## Domain model

```text
Organization
├── Campaign
│   ├── budget / daily target / rates / status
│   ├── Playbook
│   │   └── slides / captions / hashtags / slide images
│   └── Posts
└── Audit log
```

Do not invent Noise objects. A “card” usually means a playbook slide. A
“storyboard” is a planning concept unless the discovered MCP schema defines it.
Resolve the user term to a real campaign, playbook, slide, or post before a
write.

## Select the interface

```text
                    ┌─────────────────────┐
                    │ Noise task          │
                    └──────────┬──────────┘
                     interactive│automation
              ┌────────────────┴────────────────┐
              ▼                                 ▼
     ┌──────────────────┐              ┌──────────────────┐
     │ direct Noise MCP │              │ noise CLI        │
     │ native schemas   │              │ JSON + shell     │
     └──────────────────┘              └──────────────────┘
```

- Prefer direct MCP for campaign and playbook lifecycle work.
- Prefer typed REST CLI commands for stable reporting pipelines.
- Use `noise mcp tools` and `noise mcp describe <tool>` before a generic CLI
  MCP call. Do not hard-code a tool schema from memory.
- The public Reporting REST API is read-only. Do not search for a REST write
  endpoint when MCP owns the requested change.

## Read workflow

1. Discover the connected MCP tools at the start of the run.
2. Read the smallest useful scope: list first, then detail by exact ID.
3. Keep the IDs returned by Noise. Never derive an ID from a name.
4. State the date range, timezone, filters, grouping, and metric definitions.
5. Paginate until the requested result is complete. Do not treat one page as
   the full result.
6. Separate observed data from recommendations.

For reporting through the CLI:

```sh
noise report daily --start-date YYYY-MM-DD --end-date YYYY-MM-DD
noise post list --start-date YYYY-MM-DD --end-date YYYY-MM-DD
noise campaign list
noise playbook list
```

## Write workflow

Treat every Noise write as external and potentially spend-affecting.

```text
┌────────────┐   ┌────────────┐   ┌────────────────┐
│ read state │   │ preview    │   │ user approval  │
│ save IDs   │──►│ confirm=no │──►│ exact diff     │
└────────────┘   └────────────┘   └───────┬────────┘
                                          │ explicit yes
                       ┌──────────────────┴──────────────┐
                       ▼                                 ▼
              ┌────────────────┐               ┌────────────────┐
              │ apply once     │──────────────►│ readback+audit │
              │ confirm=yes    │               │ verify result  │
              └────────────────┘               └────────────────┘
```

1. Read and retain the current resource.
2. Call the discovered write tool with its preview or `confirm=false` mode.
3. Show the exact resource ID and before/after values.
4. Get explicit approval immediately before the live write. Prior approval to
   investigate or prepare is not approval to change Noise.
5. Apply one logical change once.
6. Read the resource again and inspect the audit log.

Stop instead of applying when:

- The resource ID is unclear.
- The preview differs from the requested change.
- State changed after the preview.
- The tool lacks a preview or Boolean confirmation guard.
- The change exceeds the user-approved values or scope.

If an apply result is missing, times out, or is ambiguous, do not retry. Read
the resource and audit log first. A retry can duplicate a successful write.

## CLI guarded write

The CLI previews a non-read-only MCP tool without `--apply`. It applies only
after the same call includes `--apply`:

```sh
noise mcp describe update_campaign_budget
noise mcp call update_campaign_budget --arguments '<schema-valid-json>'
# Review the preview and get approval.
noise mcp call update_campaign_budget --arguments '<same-json>' --apply
```

The CLI refuses a write-like tool that has no Boolean `confirm` input. On exit
code `9`, inspect current state and the audit log before any retry.

## Capability boundaries

- REST and MCP return the same reporting data according to Noise. Exact
  argument shapes can differ; inspect the live MCP schema.
- MCP adds campaign detail, playbook detail, audit records, status changes,
  budgets, daily targets, rates, playbook authoring, and slide-image work.
- The observed MCP does not create campaigns or expose delete, post-write,
  creator-management, card, or storyboard tools. Never simulate a missing
  lifecycle operation with a different tool.
- REST adds a dedicated unauthenticated health endpoint.
- The CLI is a transport union: typed REST reads plus a generic guarded MCP
  client. Direct MCP remains better for agent tool selection and schema use.

Primary sources:

- https://platform.getnoise.com/docs/connect-ai-client
- https://platform.getnoise.com/docs/reporting-api
- https://reporting.getnoise.com/openapi.yaml

# Issue tracker: Linear

Issues and PRDs for this repo live in Linear, team **Unbusy** (no sub-projects — everything is tracked directly on the team). Use the `claude_ai_Linear` MCP tools for all operations, never a raw API call.

## Conventions

- **Create an issue**: `save_issue` with `team: "Unbusy"` and a `title`; add `description` as Markdown (literal newlines, not `\n` escapes).
- **Read an issue**: `get_issue` by ID/identifier (e.g. `UNB-42`), plus `list_comments` with `issueId` for discussion.
- **List issues**: `list_issues` with `team: "Unbusy"`, filtered by `state`, `label`, `assignee` as needed.
- **Comment on an issue**: `save_comment` with `issueId` and `body`.
- **Apply / remove labels**: `save_issue` with `id` and `labels` — this **replaces the full label set**, so pass the union of existing + new labels (read current labels off `get_issue` first, don't just append blind).
- **Create a label** (if it doesn't exist yet): `create_issue_label` with `teamId` set to the Unbusy team, or omit `teamId` for a workspace-wide label.
- **Close**: `save_issue` with `id` and `state` set to a Done/Cancelled state name — resolve valid names first via `list_issue_statuses` with `team: "Unbusy"`.

## When a skill says "publish to the issue tracker"

Create a Linear issue on the Unbusy team via `save_issue`.

## When a skill says "fetch the relevant ticket"

Run `get_issue` on the ticket's identifier, plus `list_comments` for its discussion thread.

## Pull requests as a triage surface

Not applicable — Linear issues aren't paired with a PR-based triage flow the way GitHub/GitLab are. If a skill needs to link a PR to an issue, attach it as a link via `save_issue`'s `links` field rather than treating the PR itself as a trackable item.

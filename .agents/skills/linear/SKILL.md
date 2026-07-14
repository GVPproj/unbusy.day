---
name: linear
description: Work with Linear issues on the Unbusy team — create, read, list, comment, relabel, and close issues via a thin GraphQL wrapper (curl + jq, no MCP). Use for any issue-tracker task, or when a skill says "publish/fetch from the issue tracker".
---

# Linear

A single zero-dependency script, `linear.sh`, wraps Linear's GraphQL API. It
replaces the `claude_ai_Linear` MCP tools (which don't exist in pi — pi ships
no MCP client by design). Conventions live in `docs/agents/issue-tracker.md`;
**read it first** for team name, label-replace semantics, and close flow.

## Setup (once)

1. Create a personal API key: <https://linear.app/settings/api> → **Personal API keys** → new key.
2. Add it to the repo's gitignored env file (`.env`, already gitignored here):

   ```
   LINEAR_API_KEY=lin_api_xxxxxxxxxxxxxxxxxxxx
   ```

   The script also reads `LINEAR_API_KEY` from the environment, or `~/.config/linear.env`.
3. Sanity check:

   ```bash
   .agents/skills/linear/linear.sh team
   ```

   Should print the `Unbusy` team JSON. If it 401s, the key is wrong.

All examples below assume you're at the repo root. Default team is `Unbusy`
(override per-invocation with `--name`, or globally with `LINEAR_TEAM`).

## Operations (map 1:1 to docs/agents/issue-tracker.md)

```bash
# List / read
linear.sh list                              # all issues on Unbusy
linear.sh list --state "In Progress"
linear.sh list --label bug --assignee me
linear.sh get UNB-42                        # full detail (accepts identifier or UUID)
linear.sh comments UNB-42

# Create / discuss
linear.sh create --title "Add X" --desc "body as markdown" --labels bug,backend
linear.sh comment UNB-42 "Found the cause: ..."

# Labels — prefer add-labels unless you mean to replace the whole set
linear.sh add-labels UNB-42 triage          # unions with current labels (safe)
linear.sh set-labels UNB-42 bug,backend     # REPLACES the full set
linear.sh create-label needs-review         # on the Unbusy team
linear.sh labels                             # list available label names

# State / close
linear.sh statuses                          # resolve valid state names first
linear.sh update UNB-42 --state "In Progress"
linear.sh update UNB-42 --state Done        # close
```

## Notes

- **Never hand-roll Linear API calls** outside this script — keep the seam in one
  place. If you need an operation the script doesn't cover, add a subcommand here
  rather than shelling out to raw GraphQL inline.
- Filters and label/state args are **by name**; the script resolves names to ids
  server-side and errors with the available values if a name is unknown.
- `set-labels` replaces the entire label set; `add-labels` unions with the
  current set. The issue-tracker doc's "don't append blind" warning is exactly
  why `add-labels` exists — use it unless you deliberately mean to replace.
- Output is JSON; pipe through `jq` for selective fields, e.g.
  `linear.sh list | jq '.issues.nodes[] | {id, identifier, title}'`.

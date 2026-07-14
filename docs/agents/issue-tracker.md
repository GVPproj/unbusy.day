# Issue tracker: Linear

Issues and PRDs for this repo live in Linear, team **Unbusy** (no sub-projects — everything is tracked directly on the team). Use the `linear` skill (`.agents/skills/linear/linear.sh`) for all operations — it wraps Linear's GraphQL API over `curl`+`jq`. Never hand-roll raw Linear API calls outside the skill; if you need a new operation, add a subcommand to the script.

In pi, load it with `/skill:linear` (or just run the script). The skill's `SKILL.md` has the full command reference; this doc holds the per-operation conventions.

## Conventions

- **Create an issue**: `linear.sh create --title "…" --desc "…" --labels A,B` (description is plain Markdown).
- **Read an issue**: `linear.sh get UNB-42` (accepts an identifier or UUID), plus `linear.sh comments UNB-42` for the discussion thread.
- **List issues**: `linear.sh list --state "…" --label … --assignee me` (team is Unbusy by default).
- **Comment on an issue**: `linear.sh comment UNB-42 "body"`.
- **Apply / remove labels**: `linear.sh set-labels UNB-42 A,B` **replaces the full label set**; `linear.sh add-labels UNB-42 C` unions with the current set — prefer `add-labels` unless you deliberately mean to replace (the old "don't append blind" warning).
- **Create a label** (if it doesn't exist yet): `linear.sh create-label "name"` for the Unbusy team, or `--team <other>` elsewhere.
- **Close**: `linear.sh update UNB-42 --state "Done"` — resolve valid names first via `linear.sh statuses`.

## When a skill says "publish to the issue tracker"

Create a Linear issue on the Unbusy team via `linear.sh create`.

## When a skill says "fetch the relevant ticket"

Run `linear.sh get` on the ticket's identifier, plus `linear.sh comments` for its discussion thread.

## Pull requests as a triage surface

Not applicable — Linear issues aren't paired with a PR-based triage flow the way GitHub/GitLab are. If a skill needs to link a PR to an issue, attach it as a link — there's no dedicated subcommand yet, so add a `link` subcommand to `linear.sh` (an `issueUpdate` with a `link` input) rather than treating the PR itself as a trackable item.

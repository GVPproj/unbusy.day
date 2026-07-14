# Linear skill — progress note

Working note for the `linear` skill (`.agents/skills/linear/`). Scratch file — delete once
the work is committed. See `docs/agents/issue-tracker.md` for the repo's Linear conventions
this skill implements.

## Status: functionally complete (2026-07-14)

All 12 subcommands live-verified against the real Unbusy workspace, error paths included.
Test artifacts (UNB-27, two smoke labels) were created and deleted; workspace restored.

## Files

- `.agents/skills/linear/linear.sh` — dispatcher, 12 subcommands, executable.
- `.agents/skills/linear/SKILL.md` — skill manifest (valid: name `linear`, 242-char desc).
- `docs/agents/issue-tracker.md` — rewritten to reference `linear.sh` instead of MCP tools.

## Live test results

Reads: `team`, `statuses`, `labels`, `list` (plain / `--state` / `--label`), `get` (identifier
+ uuid), `comments`.

Writes (each verified live and reverted): `set-labels` / `add-labels` (issueUpdate),
`create` (issueCreate → UNB-27), `comment` (commentCreate), `create-label`
(issueLabelCreate), `update --state Done` (issueUpdate + state resolution).

Error paths (all exit 1 with a single clean message): `get UNB-99999` (issue not found),
`update --state Nonexistent` (lists available states), `add-labels NotARealLabel` (lists
available labels).

## Linear schema gotchas discovered + fixed

Recorded so they aren't re-discovered:

1. **`id` args are `String!`, not `ID!`.** `team(id:)`, `issue(id:)`, `issueUpdate(id:)` all
   reject `ID!` variables. Fixed: every variable annotation is `String!`.
2. **`PaginationOrderBy` has only `createdAt` / `updatedAt`** — no `*DESC`. DESC is the
   default for dates. Fixed: `orderBy: updatedAt`.
3. **`IssueFilter` has no `identifier` field.** Resolve `UNB-42` by splitting `<KEY>-<num>`
   and filtering `{number:{eq:$n}, team:{key:{eq:$k}}}`.
4. **`NumberComparator.eq` is `Float`.** An `Int!` variable won't coerce to a `Float` arg
   (GraphQL variable-type checking is stricter than literal coercion). Use `$n:Float!`.
5. **`issueLabelCreate` without `teamId` makes a workspace label.** Fixed: `create-label`
   now always resolves a team (default Unbusy), matching its documented behavior.
6. **bash: `set -e` doesn't catch failures inside `$(...)`.** Every `gql` and jq-extractor
   capture in the resolvers carries an explicit `|| exit 1`.

## Remaining step

Commit `.agents/skills/linear/` + the `docs/agents/issue-tracker.md` rewrite (CLAUDE.md:
open a Linear issue first, sign off with `git commit -s`), then delete this file.

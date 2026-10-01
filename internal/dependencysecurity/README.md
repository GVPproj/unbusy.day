# Browser dependency security (UNB-76)

Run from the repository root; Go standard library only:

```sh
go run ./internal/dependencysecurity                 # default: integrity
go run ./internal/dependencysecurity -mode integrity # offline
go run ./internal/dependencysecurity -mode advisories
go test -race ./internal/dependencysecurity
```

Both modes exit nonzero on invalid/missing inventories or check failures.
Neither updates vendor files. No templ generation or Node tooling is needed.

## Integrity

Verifies every CodeMirror manifest module's **served-file** `sha256`, and every
entry in Datastar's `SHA256SUMS`. Rejects empty inventories, duplicate/unsafe
paths, symlink entries/parents, non-regular files, and invalid checksums. Both
modes reject CodeMirror `.js`/`.mjs` files absent from the manifest. The
local bundle pinned by `internal/frontend/layouts/datastar.templ` must be covered
by `SHA256SUMS`.

This detects drift from committed records, not malicious changes to both an
artifact and its checksum. It is not upstream signature verification, a check
of CodeMirror's pre-transformation `sourceSHA256`, or an audit of module imports.

## Advisories

- Deduplicates CodeMirror's deployed npm package/version pairs (excluding
  `version: "builtin"`) and POSTs batches of at most 1,000 to OSV's
  `/v1/querybatch`. Prints each affected pair and vulnerability ID.
- Reads all published repository security advisories from GitHub's
  `/repos/starfederation/datastar/security-advisories`, using numbered pages of
  100. Set `GITHUB_TOKEN` optionally to increase API rate limits; the token is
  sent only to GitHub, not OSV. No dotenv loading is performed.
- **Conservative Datastar limitation:** every published repository advisory
  fails the check and requires manual applicability review, even if the pinned
  version is already fixed or the advisory concerns another artifact. No
  package mapping or version-range assumptions are made; withdrawn published
  entries are not automatically dismissed.

Both sources are checked even if one fails or reports findings. Requests have a
30-second timeout; HTTP errors (including rate limits), malformed/incomplete
responses, and OSV continuation tokens fail closed rather than reporting clean.
GitHub pagination is bounded at 10,000 pages and repeated advisories fail closed.
OSV batch results containing continuation tokens require manual follow-up; this
command does not silently treat truncated vulnerability lists as complete.

A clean result means these sources reported no findings, not that dependencies
are vulnerability-free. Registry coverage and upstream availability are outside
this tool's control. Keep online advisory monitoring separate from deployment
integrity gates. This command does not check version freshness or automatically
refresh, fix, suppress, or merge dependencies.

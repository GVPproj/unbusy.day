# CodeMirror vendor graph

The Jotpad loads CodeMirror only from
`internal/frontend/static/vendor/codemirror/`. The exact modules in `roots` in
`internal/frontend/vendorcodemirror/main.go` are the direct upgrade inputs. The
generated manifest is the deployed lock state: it records the exact root set
and the complete resolved graph's package versions, source URLs, source
SHA-256s, and served-file SHA-256s. Each served module includes its upstream
MIT notice.

## Reproduce the locked graph

Run:

```sh
task vendor:codemirror
```

The default command requires an existing manifest. It uses the deployed
versions in that lock to resolve transitive ranges, then verifies both every
fetched source hash and every final served-file hash before replacing the
vendor directory. The final hash also locks license text and transformation
output. Missing modules, changed same-version content, and roots that do not
match the manifest fail with `-refresh` guidance while leaving committed assets
intact.

The command fetches exact esm.sh modules and npm license files, rewrites imports
to local paths, removes source-map references, and removes Lezer's Node-only
parse-debug switch. An unchanged default run must produce no diff. A missing
manifest does not bootstrap implicitly; use `-refresh` for first generation.

## Upgrade

1. Change the six direct versions in `roots` at the top of
   `internal/frontend/vendorcodemirror/main.go`.
2. Run `go run ./internal/frontend/vendorcodemirror -refresh` to resolve a new
   transitive graph and intentionally write new content hashes. Stable local
   filenames keep `js/jot/cm.js` unchanged.
3. Review all generated module, manifest version, and hash changes.
4. Run `go test ./internal/frontend` and `task test:browser`.

`-refresh` is the intentional update path: it accepts new source, license, and
transformation bytes as well as resolving transitive versions again. The
default command is verification, so a normal reproduction cannot silently
adopt a compatible release or changed same-version artifact. `task
check:versions` reads the generated manifest because it reports the complete
graph actually deployed, not only the direct upgrade inputs.

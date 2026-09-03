# CodeMirror vendor graph

The Jotpad loads CodeMirror only from
`internal/frontend/static/vendor/codemirror/`. Direct version inputs live in
the vendor command; its generated manifest records the resolved package
version, source URL, source SHA-256, and served-file SHA-256 for every module.
Each served module includes its upstream MIT notice.

## Reproduce the locked graph

Run:

```sh
task vendor:codemirror
```

The Go command uses the package versions in the current manifest to resolve
all transitive ranges. It fetches exact esm.sh modules and npm license files,
rewrites imports to local paths, removes source-map references, removes
Lezer's Node-only parse-debug switch, and replaces the vendor directory. A
second run must produce no diff.

## Upgrade

1. Change the six direct versions in `roots` at the top of
   `internal/frontend/vendorcodemirror/main.go`.
2. Run `go run ./internal/frontend/vendorcodemirror -refresh` to resolve a new
   transitive graph. Stable local filenames keep `js/jot/cm.js` unchanged.
3. Review all manifest version and hash changes.
4. Run `go test ./internal/frontend` and `task test:browser`.

`-refresh` is an explicit network update. The default command keeps the
current transitive versions, so a normal reproduction cannot silently adopt a
new compatible release.

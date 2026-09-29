# Datastar browser runtime

The app and `/_smoke` share `layouts.Datastar`, whose versioned local URL is
our client pin. These files are unmodified upstream release artifacts (the
bundle is renamed for cache identity); no Node build or CDN is needed at runtime.
The source map retains its upstream name to match the bundle's sourceMappingURL.

## Upgrade

1. Read the release notes at <https://github.com/starfederation/datastar/releases>.
2. Download `bundles/datastar.js`, `bundles/datastar.js.map`, and `LICENSE.md`
   from `https://raw.githubusercontent.com/starfederation/datastar/v<VERSION>/`.
   Save the bundle here as `datastar-<VERSION>.js`, and the other files with
   their original basenames. Remove the previous bundle.
3. Update the URL in `internal/frontend/layouts/datastar.templ`.
4. From this directory, record the reviewed artifacts with
   `sha256sum datastar-<VERSION>.js datastar.js.map LICENSE.md > SHA256SUMS`.
   `sha256sum -c SHA256SUMS` verifies the committed bytes offline.
   On macOS, substitute `shasum -a 256` for `sha256sum`.
5. Run `task check:versions` and `task test:browser` (includes the element-patch
   and underscore-signal/POST-echo wiring canary). Run `task test` too.

Current artifacts were downloaded from the `v1.0.4` tag; the bundle was also
byte-compared with the release's jsDelivr URL. `LICENSE.md` carries the upstream
MIT license and copyright notice. `SHA256SUMS` is a local integrity record, not
an upstream signature.

# Cloudflare cache rules (PRD D3 / M3b)

`cache-rules.json` is the canonical, version-controlled definition of the three
Cache Rules in front of `hello-cards.fly.dev` at the apex `unbusy.day`. It is the
body for a `PUT` against the zone's `http_request_cache_settings` ruleset
entrypoint.

**Ordering matters.** Cloudflare cache rules are *last-match-wins*: when several
rules match a request, the last one's settings override earlier ones. So the
catch-all `true` rule (respect origin) is **first**, and the two bypass rules
(`/api/*`, `/events`) come **after** it so they win for those paths.

- (c) default → `cache: true` + `respect_origin`: honours origin `Cache-Control`
  (`/assets/*` is `immutable` year-long; `index.html` is `no-cache` so the edge
  revalidates the entry point — F4).
- (b) `/api/*` → `cache: false`.
- (a) `/events` suffix → `cache: false` (covers `/api/events` **and**
  `/ds/events`). SSE stays un-buffered because it is never cached; the origin
  also sets `Cache-Control: no-cache` + `X-Accel-Buffering: no` (F2) and the Go
  server disables `WriteTimeout` for the stream.

## Apply

```bash
export CLOUDFLARE_API_TOKEN='<zone-scoped: My Profile > API Tokens>'  # gitleaks:allow
ZONE=unbusy.day
ZONE_ID=$(curl -s -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones?name=$ZONE" | jq -r '.result[0].id')

curl -X PUT \
  "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/rulesets/phases/http_request_cache_settings/entrypoint" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" -H 'Content-Type: application/json' \
  --data @ops/cloudflare/cache-rules.json
```

## Verify (criterion 6 — no token needed)

```bash
curl -I https://unbusy.day/assets/<hashed>.js   # cf-cache-status: HIT (warm) + immutable
curl -I https://unbusy.day/api/cards            # cf-cache-status: DYNAMIC (or BYPASS)
curl --no-buffer -N https://unbusy.day/api/events   # event-by-event, not buffered
curl --no-buffer -N https://unbusy.day/ds/events    # ditto
```

> **Provenance:** the live zone was configured via the Cloudflare dashboard and
> criterion 6 was verified end-to-end with the curl checks above (assets `HIT`,
> `/api/*` `DYNAMIC`, both SSE streams un-buffered, observed from colo `YVR`).
> This file is the reproducible spec of that state; it was authored to the
> Cloudflare cache-rules schema, not byte-captured from the live ruleset (that
> read needs `CLOUDFLARE_API_TOKEN`, absent on the deploy machine). Re-applying
> it reproduces the verified behaviour.

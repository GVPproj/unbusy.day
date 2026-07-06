# Cloudflare cache rules

`cache-rules.json` is the canonical, version-controlled definition of the two
Cache Rules in front of `hello-cards.fly.dev` at the apex `unbusy.day`. It is the
body for a `PUT` against the zone's `http_request_cache_settings` ruleset
entrypoint.

**Ordering matters.** Cloudflare cache rules are *last-match-wins*: when several
rules match a request, the last one's settings override earlier ones. So the
catch-all `true` rule (respect origin) is **first**, and the `/events` bypass
rule comes **after** it so it wins for that path.

- (b) default → `cache: true` + `respect_origin`: honours origin `Cache-Control`.
  The frontend is server-rendered and served `no-cache`, so the edge
  revalidates the entry document on every hit. Static runtimes (Datastar,
  Motion) load from jsdelivr, not the origin.
- (a) `/events` suffix → `cache: false`. SSE stays un-buffered because it is
  never cached; the origin also sets `Cache-Control: no-cache` +
  `X-Accel-Buffering: no` and disables `WriteTimeout` for the stream.

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

## Verify (no token needed)

```bash
curl -I https://unbusy.day/                      # cf-cache-status: DYNAMIC (origin no-cache)
curl --no-buffer -N https://unbusy.day/events    # event-by-event, not buffered
```

> **Provenance:** the live zone was configured via the Cloudflare dashboard and
> verified end-to-end with the curl checks above. This file is the reproducible
> spec of that state, authored to the cache-rules schema rather than
> byte-captured from the live ruleset.

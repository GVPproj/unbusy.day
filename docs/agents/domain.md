# Domain Docs

How agents should consume this repo's domain documentation.

- **Read `CONTEXT.md`** (the glossary) and any `docs/adr/` entries that touch the
  area you're about to work in.
- **Use the glossary's vocabulary** when your output names a domain concept — in
  an issue title, a proposal, a test name. Don't drift to synonyms. A concept
  that isn't in the glossary is a signal: either you're inventing language the
  project doesn't use, or there's a real gap worth recording.
- **Flag ADR conflicts** rather than silently overriding one:
  _"Contradicts ADR 0007 (SQLite as the source of truth) — but worth reopening
  because…"_

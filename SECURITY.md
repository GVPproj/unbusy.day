# Dependency security

## Owner and reporting

The repository maintainer (@GVPproj) owns dependency triage and upgrades. Enable
GitHub Actions failure notifications for **Dependency security** and **CI/CD**;
check the latest scheduled results weekly, including missed or disabled runs.
Automation reports in Actions, not Linear. Create a Linear issue for actionable
findings, recording affected deployed versions, applicability, owner, and target
date. Record any deferral with its rationale and next review date.

Report suspected vulnerabilities privately through GitHub's **Security → Report
a vulnerability**, if enabled. If unavailable, ask the maintainer for a private
contact channel without publishing exploit details in an issue.

## Three separate checks

- **Integrity — every CI run, offline:** `go run ./internal/dependencysecurity
  -mode integrity` verifies the served CodeMirror modules against their manifest
  hashes and Datastar artifacts against `SHA256SUMS`, including coverage of the
  pinned browser bundle. This gates deployment. Locally recorded hashes detect
  unexpected changes; they are not upstream signatures or proof of safe code.
- **Known advisories — daily and manually:** the separate Dependency security
  workflow queries OSV for every deployed npm package/version in CodeMirror's
  manifest, including transitives. It also checks Datastar's GitHub repository
  security advisories. Any published Datastar advisory requires manual
  applicability review: this check does not interpret its affected-version
  ranges, so even historical/fixed advisories can keep the job red. API failures
  also fail the check; unavailable or malformed results are not a clean scan.
- **Freshness — Mondays and manually:** `task check:versions` compares pinned
  versions with upstream releases. Drift fails this monitoring job, not the
  deployment gate, and is not itself evidence of a vulnerability.

The monitor reads the deployed CodeMirror manifest, not `package-lock.json`,
which describes Node test tooling. Advisory coverage is imperfect: during weekly
triage also review [Datastar releases](https://github.com/starfederation/datastar/releases)
and [CodeMirror releases](https://codemirror.net/docs/changelog/) for security
fixes that have no advisory. These checks do not cover Go dependencies, Node test
tooling, GitHub Actions, or the container base image; those remain separate
maintenance responsibilities. No findings does not establish that code is safe.

## Cadence and response targets

- Triage security alerts within one working day of notification.
- For actively exploited or applicable critical vulnerabilities, aim to deploy a
  fix or mitigation within 24 hours of awareness; applicable high-severity
  vulnerabilities within seven days. Escalate immediately if these targets cannot
  be met, documenting the mitigation and remaining exposure in Linear.
- Review version drift weekly and batch routine upgrades monthly. Assess lower
  severity findings for that maintenance window; defer only with a recorded
  reason and review date.

## Accepting new dependency bytes

Follow the [CodeMirror upgrade procedure](docs/agents/codemirror.md) and
[Datastar upgrade procedure](internal/frontend/static/vendor/datastar/README.md).
Review release notes, provenance, new transitives, and generated diffs. CodeMirror
currently trusts esm.sh's transformed modules and npm license files at acquisition
time; content locking prevents later silent changes, not a compromised initial
artifact. Datastar is acquired from its upstream tagged artifacts.

Never run unattended CodeMirror `-refresh`, silently replace mismatched hashes,
or auto-merge vendor updates. A mismatch requires investigation; a deliberate
upgrade requires review. Keep vendor-only changes separate where practical.

Run `task test` and the full `task test:browser` suite before accepting an update;
CI runs the full browser suite for vendor-update PRs as well as nightly. Check
Datastar patch/signal wiring and Jotpad editing/sync. Verify that an already-open
client and a subsequent reload receive compatible assets, including service-worker
cache behavior and CodeMirror's stable module filenames. The current service
worker is passthrough-only; normal browser HTTP caching still matters. Do not assume browser
tests alone prove an existing client's caches have been invalidated.

Monitoring never updates dependencies or deploys fixes automatically. Its network
availability is intentionally independent of the deployment gate. If a browser
runtime is compromised, assess exposure of authenticated data/actions too;
HttpOnly session cookies do not prevent same-origin malicious scripts acting as
the user.

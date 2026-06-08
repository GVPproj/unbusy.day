# Contributing

Thanks for your interest in contributing. Contributions are welcome.

## License & what this is

This project is **source-available** under the
[Functional Source License, FSL-1.1-Apache-2.0](./LICENSE.md) — not OSI "open
source." You can read, modify, and build on it for any purpose **except**
offering it as a commercial product or service that competes with the
licensor's hosted offering. Each release converts to Apache-2.0 two years after
it is published. The hosted service is the commercial offering; the source is
here so you can self-host, learn from it, and contribute.

By contributing, you agree your contribution is licensed to the project and its
users under these same terms.

## Developer Certificate of Origin (DCO)

We use the [Developer Certificate of Origin](https://developercertificate.org/)
instead of a CLA — no forms to sign. You certify the DCO by adding a
`Signed-off-by` line to every commit:

```
git commit -s -m "Your message"
```

This appends a line like:

```
Signed-off-by: Your Name <your.email@example.com>
```

Use your real name and an email you can be reached at. The git author must match
the sign-off. A CI check enforces sign-off on every commit in a pull request;
PRs with unsigned commits will fail until fixed.

Forgot to sign off? Amend the last commit with `git commit --amend -s`, or for a
whole branch: `git rebase --signoff main` (then force-push your branch).

<details>
<summary>Full DCO text (version 1.1)</summary>

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

</details>

## Submitting changes

1. Open an issue first for anything non-trivial so we can agree on the approach.
2. Branch, make your change, keep commits focused, and sign them off (`-s`).
3. Open a pull request describing the change and how you tested it.

## Don't commit secrets

Never commit real credentials. Secrets are injected at deploy/runtime
(`fly secrets`, env vars, GitHub repo secrets) — see PRD §13. Local config goes
in `.env` (git-ignored); copy `.env.example` if present.

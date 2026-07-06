# Context — hello-cards

Glossary of the domain language. Definitions only — no implementation detail.

## User
A person who can log in. Identified by their **email address** (the email *is*
the identity; there are no usernames or passwords). A User privately owns a
Day Plan.

## Block
A single time block on a User's Day Plan. Owned by exactly one User. Occupies a
contiguous run of Slots: a starting Slot plus a span (number of Slots tall).
A Block is anchored to the clock, not to the top of the plan — changing the
Day Plan's bounds never moves a Block's time.

## Block Type
The nature of a Block, chosen by the User at creation: **deep** (demanding,
focused work), **shallow** (low-cognitive-demand work), or **break** (rest, not
work). A flat three-way tag — deep and shallow are grades of work and break is
its absence, but all three are treated as peer values of one attribute. Every
Block has exactly one Block Type. Color-coded on the Day Plan.

## Day Plan
A User's schedule for one day, in the Cal Newport time-blocking style. Each
User has exactly one, private and never shared — it is a rolling "today," not
tied to a calendar date (no history of past days). Has a start time and an end
time on half-hour boundaries, set and modified by the User; the number of
Slots is derived from those bounds, never set directly. Bounds lie within a
single day — no midnight wrap — starting no earlier than 4:00 and ending no
later than 18:00 (for now). The day can only shrink into empty Slots: a
bounds change that would leave any Block outside the Day Plan is rejected.
The User's current time of day is indicated live on the plan.

## Template (future)
A reusable Day Plan layout a User can stamp onto their Day Plan. Not built
yet; named here so "one rolling Day Plan" isn't read as "one layout forever."

## Slot
One 30-minute interval within a Day Plan, identified by the time it begins. A
Slot is either empty or covered by exactly one Block — Blocks never overlap.

## Push
What happens when a Block is placed (moved or grown) onto occupied Slots: each
overlapped Block is pushed *down* (later in the day) by the minimum distance
that clears the overlap, consuming empty Slots before displacing further
Blocks. A placement whose push would force any Block past the end of the Day
Plan is rejected whole — nothing moves.

## Allowlist (retired)
Historical: login was once gated to existing Users — an email with no User row
could not obtain a Login Code. That gate is **gone**. Signup is open: any
deliverable email can request a Login Code, and verifying a correct code mints
the User on first login. The blast-radius bound is now a layered set of send-path
defenses (per-email throttle, suppression list, MX check, Turnstile presence
check, per-IP/global rate limit, global send ceiling) rather than an allowlist.

## Login Code
A short-lived, single-use numeric code emailed to a User to prove control of
their email address. Verifying a valid Login Code establishes a Session. (Also
called OTP — one-time passcode.)

## Session
Proof that a request comes from a logged-in User. Established by verifying a
Login Code, ended by logging out or by expiry. A Session can be revoked
server-side at any time (logout is immediate and authoritative).

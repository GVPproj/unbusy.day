# Context — hello-cards

Glossary of the domain language. Definitions only — no implementation detail.

## User
A person who can log in. Identified by their **email address** (the email *is*
the identity; there are no usernames or passwords). A User privately owns a
Day Plan.

## Card
A single item on a User's Day Plan. Owned by exactly one User. Occupies a
contiguous run of Slots: a starting Slot plus a span (number of Slots tall).
A Card is anchored to the clock, not to the top of the plan — changing the
Day Plan's bounds never moves a Card's time.
(Legacy meaning — a dense position ordering Cards top-down — is being replaced
by Slot placement.)

## Day Plan
A User's schedule for one day, in the Cal Newport time-blocking style. Each
User has exactly one, private and never shared — it is a rolling "today," not
tied to a calendar date (no history of past days). Has a start time and an end
time on half-hour boundaries, set and modified by the User; the number of
Slots is derived from those bounds, never set directly. Bounds lie within a
single day — no midnight wrap — starting no earlier than 5:00 and ending no
later than 18:00 (for now). The day can only shrink into empty Slots: a
bounds change that would leave any Card outside the Day Plan is rejected. Eventually the User's current time of day will be
indicated live on the plan. (Replaces the retired term **Board**.)

## Template (future)
A reusable Day Plan layout a User can stamp onto their Day Plan. Not built
yet; named here so "one rolling Day Plan" isn't read as "one layout forever."

## Slot
One 30-minute interval within a Day Plan, identified by the time it begins. A
Slot is either empty or covered by exactly one Card — Cards never overlap.

## Push
What happens when a Card is placed (moved or grown) onto occupied Slots: each
overlapped Card is pushed *down* (later in the day) by the minimum distance
that clears the overlap, consuming empty Slots before displacing further
Cards. A placement whose push would force any Card past the end of the Day
Plan is rejected whole — nothing moves.

## Allowlist
The set of Users permitted to log in. Currently the Allowlist *is* the set of
existing Users: you become allowed by being added as a User. An email with no
User is not on the Allowlist and cannot obtain a Login Code. (This equivalence
is temporary — at public launch, requesting a code will create the User, opening
login to anyone.)

## Login Code
A short-lived, single-use numeric code emailed to a User to prove control of
their email address. Verifying a valid Login Code establishes a Session. (Also
called OTP — one-time passcode.)

## Session
Proof that a request comes from a logged-in User. Established by verifying a
Login Code, ended by logging out or by expiry. A Session can be revoked
server-side at any time (logout is immediate and authoritative).

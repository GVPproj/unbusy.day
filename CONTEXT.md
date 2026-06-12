# Context — hello-cards

Glossary of the domain language. Definitions only — no implementation detail.

## User
A person who can log in. Identified by their **email address** (the email *is*
the identity; there are no usernames or passwords). A User privately owns a
Board.

## Board
The ordered set of Cards belonging to one User. Each User sees and reorders only
their own Board; Boards are private and never shared. ("Board" is the domain
concept; today it is implicit — a User's Cards — not a separate named entity.)

## Card
A single item on a User's Board. Owned by exactly one User. Has a position
(order within the Board) and a span (height in slots).

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

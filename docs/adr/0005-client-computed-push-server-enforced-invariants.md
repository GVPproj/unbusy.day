# Client-computed Push, server-enforced invariants

Status: accepted

Moving from list reorder to Slot placement on the Day Plan, the Push cascade
(displaced blocks slide toward the slot the moved block vacated — down-drag
pushes others up, up-drag pushes them down — consuming gaps) could live
server-side (client
sends intent: block + target slot) or client-side (client computes the
resulting layout and sends it whole, as the permutation reorder it replaces did).
We chose client-side: the block gesture modules (`js/blocks/push.js`, driven by
`pointer.js` and `keyboard.js`) compute Push so the optimistic FLIP commit can
render the true outcome instantly, and the server validates only the invariants
— same block set, in bounds, no overlaps, span ≥ 1 — which it enforces exactly
once in `SetLayout`. This is a deliberate, feel-driven exception to "no
client-side business logic": any layout satisfying the invariants is a state
the User could have reached by gestures, so the server's authority over legal
states is undiminished.
Revisit if a server-intent round trip can be made to feel as good — interaction
feel is mission critical and is the reason for this exception.

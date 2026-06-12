# Client-computed Push, server-enforced invariants

Moving from list reorder to Slot placement on the Day Plan, the Push cascade
(displaced cards slide down, consuming gaps) could live server-side (client
sends intent: card + target slot) or client-side (client computes the
resulting layout and sends it whole, as the permutation reorder does today).
We chose client-side: drag.js computes the Push so the optimistic FLIP commit
can render the true outcome instantly, and the server validates only the
invariants — same card set, in bounds, no overlaps — which it enforces exactly
once. This is a deliberate, feel-driven exception to "no client-side business
logic": any layout satisfying the invariants is a state the User could have
reached by drags, so the server's authority over legal states is undiminished.
Revisit if a server-intent round trip can be made to feel as good — interaction
feel is mission critical and is the reason for this exception.

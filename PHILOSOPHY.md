# Philosophy

*why Etch exists, and what it refuses to become.*

---

## the reader is not human

Every session Etch captures is written for the next agent, not for a person
at a dashboard. This inverts the entire observability instinct. The dominant
move in agent tooling is to build a screen for a human to *watch* the agents —
but at fleet scale no human watches eighty concurrent sessions, and the data
was never really for them. The next agent starting work in a codebase is the
consumer. It needs to know what came before: what was tried, what touched which
files, what the last session concluded. Etch is memory for that agent. Design
every field by asking what the next mind — silicon, not carbon — needs to read.

---

## permanence over surveillance

Etch *etches*. A session record is an immutable git object the moment it lands,
carved into a ref that travels with push and fetch and is never rewritten.
This is deliberate. Observability is a stream you watch and forget; a record is
a thing you can return to years later and trust. We are not building a feed. We
are building a permanent, queryable substrate that accumulates value as it
grows. Immutability is not a constraint we tolerate — it is the point.

---

## infrastructure, not product

Etch is infrastructure you operate and open-source as proof — not a product to
be marketed into a seat count. Know what kind of thing you have built. The
reflex to run a launch motion on every working tool is a category error; some
tools are foundations, and a foundation's value accrues to what stands on it,
not to what you charge for it. Etch's value is the fleets it serves and the
credibility it carries — that Stage 11 operates at a scale most tools have
never been asked to survive. Judge it there, not by adoption curves. This frees
the project to be honest, lean, and unhurried: it does not need a market to
justify its existence.

---

## the sovereign lane

No cloud. No account. No telemetry. No control plane in the loop. Just git refs
you already know how to push, fetch, and grep — on hosts you already control.
This is a moat, not an ideology. A well-funded incumbent on an open-core model
*structurally cedes* this ground: monetizing the convenience layer means it can
never give away the no-cloud, no-account version without bleeding its own
revenue. So own the lane it cannot follow you into. The sovereign stance is
also where the architecture comes from — per-session refs, not a shared branch,
because flat immutable writes have zero contention at any concurrency, and
because nobody should have to trust a server to remember what their own agents
did.

---

## build on open ground; don't rebuild it

Etch stands on a substrate it did not build — an open hook protocol that already
integrates the agent runtimes. Re-implementing that across eight runtimes would
buy zero differentiation; it is cost masquerading as control. Stand on open
ground and spend your effort where you are actually different. The insurance for
depending on someone else's substrate is its license: a permissive license means
*"make our own copy"* always resolves to *fork it if they turn hostile*, never
*rebuild it from scratch*. Hold that option cheaply. Do not exercise it
preemptively. Favor permissive dependencies precisely so the escape hatch stays
free.

---

## flat, and let structure emerge

Records carry no hierarchy. A session is a flat fact with shared identifiers —
ticket, run, machine, branch — and structure is composed at query time, not
imposed at write time. Hierarchies encode the questions you thought to ask when
you wrote the schema; flat records with good identifiers answer the questions
you haven't thought of yet. Capture honestly and densely. Let the shape come
later, from whoever is asking.

---

*Etch captures. It does not analyze, dashboard, or judge. Those are downstream
consumers of an honest record — and an honest, permanent record is the whole
contribution.*

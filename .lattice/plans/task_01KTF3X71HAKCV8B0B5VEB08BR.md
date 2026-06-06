# ETCH-41: Implement local_only_fields: strip-before-push transport for session refs

DECISION MADE (operator, 2026-06-06): implement for real (not soften docs). Configured local_only_fields must actually stay off the wire.

Design space (delegator plans, operator-approved direction): projection layer at push time — e.g. a parallel public ref namespace (refs/etch/public/<ULID>) holding the stripped record, with setup-refspec pushing ONLY the public namespace; or a pre-push hook rewriting outgoing refs. The local ref keeps full fidelity. Immutability holds per-namespace.

Constraints:
- Coordinates tightly with the refspec/sync batch (ETCH-16/18/24/38) — same setup-refspec surface; land AFTER that batch.
- Coordinates with ETCH-40 finding 5 (whole-record redaction pass) — the projection should reuse the same record-walking machinery.
- README/settings docs updated to describe the real behavior; until this lands, README marks local_only_fields 'in development' (ETCH-40 finding 6 interim).
Origin: ETCH-40 finding 6 / superseded ETCH-31. Review file: reviews/2026-06-04-deep-code-review.md.

# ETCH-44: install-hooks should write the .etch gitignore block

Found during the Etch rollout (ROLLOUT wave 0.5). Today every repo that enables Etch must hand-add the .etch gitignore block (ignore .etch/*, carve out !.etch/settings.json) by copying it from the Etch repo's own .gitignore. install-hooks should do this automatically so the enable checklist is one fewer manual step.

What it does: when 'entire-agent-etch install-hooks' runs, ensure the repo's root .gitignore contains the etch block. Idempotent — detect an existing block and do not duplicate; create .gitignore if absent; append (don't clobber) if present.

The block to write:
  # Etch local WIP buffers — session records live in git refs, never as files.
  # settings.json is carved out: it carries the per-repo hostname salt and is
  # committed on PRIVATE repos so all clones share it.
  .etch/*
  !.etch/settings.json

Acceptance criteria:
- install-hooks writes the block to ./.gitignore when missing.
- Re-running install-hooks does not duplicate the block (idempotent).
- Existing .gitignore content is preserved (append, never overwrite).
- The hooks_installed JSON response is unchanged in shape; gitignore handling is best-effort and never fails the install.
- Unit test in the testutil temp-repo harness covers: no .gitignore -> created; existing unrelated .gitignore -> block appended once; re-run -> no change.

Context: this is part of the 'committed hooks are the install unit' practice (ROLLOUT 'Practices being set' #1/#2). The gitignore block is currently item 2 of the per-repo enable checklist in code/platform/etch.md.

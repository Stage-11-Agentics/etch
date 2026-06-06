# ETCH-22: setup-refspec fetch refspec omits leading '+' that README's manual-equivalent shows

'setup-refspec' writes remote.origin.fetch = 'refs/etch/sessions/*:refs/etch/sessions/*' (no leading '+'), but the README's 'Equivalent manual config' shows '+refs/etch/sessions/*:refs/etch/sessions/*' (force/non-fast-forward allowed). Doc vs implementation mismatch. Harmless for immutable per-session refs, but the two should agree. Pick one and make both match.

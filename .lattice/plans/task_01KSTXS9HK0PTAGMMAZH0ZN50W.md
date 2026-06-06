# ETCH-31: local_only_fields is documented but completely unimplemented (false privacy guarantee)

AUDIT ITEM 7 (doc audit) + privacy. README.md 'Privacy & security' states: 'local_only_fields lets you keep selected fields out of any ref that gets pushed to a remote.' settings.json example documents the field.
ACTUAL: grep shows config/config.go:13 declares the struct field 'LocalOnlyFields' and NOTHING ELSE references it -- no stripping logic exists anywhere in internal/ or cmd/. Configured fields are pushed to the remote regardless. An operator relying on this to keep sensitive fields local is silently exposed. FIX: implement field stripping on push (or a pre-push transform), or remove the claim from README + settings docs until implemented.

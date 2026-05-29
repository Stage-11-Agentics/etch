# ETCH-13: Productization: README, Makefile, install path, rename Python PoC

Write a user-facing README.md (install, configure, usage, integration with Entire CLI), a Makefile (build/test/install/clean), document the install path (go install / make install to ~/.local/bin or /usr/local/bin), rename ./entire-agent-cairn (Python PoC) to ./entire-agent-cairn-poc to avoid conflict with the built Go binary, and provide a smoke-test shell script that exercises an end-to-end session against a real Entire CLI install.

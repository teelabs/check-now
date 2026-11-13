# Standalone Distribution Release Pass Plan

> **For agentic workers:** This plan is executed by the current agent; it is a bounded release review and regression fix.

**Goal:** Prepare the standalone distribution change for a local release readiness decision.

**Architecture:** Verify the existing release build and CI contract, reproduce and fix installer rollback data loss, then run repository and distribution checks. Version the feature as the next MINOR release while keeping external publication and deployment behind user approval.

**Tech Stack:** Go, POSIX shell, GitHub Actions, Cloudflare Pages.

**Spec:** GitHub issue #4 and the user-confirmed distribution decisions in this conversation.

## Global Constraints

- Linux amd64 and arm64 release bundles.
- GitHub Releases host binaries and SHA-256 checksums.
- Cloudflare Pages serves `https://check-now.pages.dev/install.sh`.
- Client installation requires only shell and common download/archive/checksum utilities.
- Do not publish, deploy, push, or tag without explicit authorization.

## Review Focus

- Failed backup of existing shared files preserves the previous installation.
- Release artifacts contain the expected version, executable, plugins, and shell assets.
- Installer checksums are validated before changing installed files.
- GitHub workflows scope credentials and deploy only the installer payload.
- The version and changelog describe the release consistently.

### Task 1: Installer rollback regression

- [x] Add a shell regression that forces the second backup move to fail and checks both prior files survive.
- [x] Run it red, fix rollback to delete only newly installed paths, then run it green.

### Task 2: Release validation

- [x] Review CI and workflow configuration against the project decisions.
- [x] Update version and changelog for the MINOR feature release.
- [x] Run unit, race, vet, plugin, shell syntax, and release build/verification checks.
- [x] Report readiness and stop before external integration or publication.

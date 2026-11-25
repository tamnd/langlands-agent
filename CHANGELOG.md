# Changelog

All changes that a user can see are in this file. The format follows Keep a Changelog, and the versions follow Semantic Versioning.

## Unreleased

### Added

- `lgl port check`, which checks the licence header and the provenance line of each ported Lean file against `manifests/ported.yaml` of langlands-lean.
- The status `faithful` of spec 08.3, for a statement that passed the fidelity checks of its class in `records/fidelity.json`.

## v0.0.1 (2026-11-25)

### Added

- `lgl check`, `lgl status`, `lgl index` and `lgl site`, adapted from singularity-agent for the map of tamnd/langlands.
- The tiers T0 to T10 of the corpus, the node kinds `check` and `claim`, and the status `claimed` for a fresh claim of route R1.
- The site pages of the program, the map, the statements, the trust report and the lab.

### Changed

- The tier titles follow spec 07, and the default trust questions Q0 to Q8 follow spec 14.
- The smoke test for exit code 3 uses `lgl agent`.

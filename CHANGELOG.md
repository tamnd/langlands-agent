# Changelog

All changes that a user can see are in this file. The format follows Keep a Changelog, and the versions follow Semantic Versioning.

## Unreleased

## v0.5.0 (2026-11-27)

### Changed

- This release closes milestone S4, Modularity and Galois representations.

## v0.4.1 (2026-11-27)

### Added

- Lab experiment E5 compares the a_p of each elliptic curve of conductor N ≤ 1000 with the newform data in Cremona's ecdata, and lists the classes that reach the Sturm bound and the Livne prime sets.

## v0.4.0 (2026-11-27)

### Changed

- This release closes milestone S3, Groups and smooth representations. It has no other change since v0.3.0.

## v0.3.0 (2026-11-27)

### Changed

- This release closes milestone S2, Global CFT and Tate's thesis. It has no other change since v0.2.0.

## v0.2.0 (2026-11-27)

### Changed

- This release closes milestone S1, Local GL(1). It has no other change since the last patch release.

## v0.1.3 (2026-11-27)

### Added

- Lab experiment E2 in `lab/e2.go`: the explicit local Langlands correspondence for $GL_2(\mathbb{Q}_p)$ with $p$ in 3, 5 and 7, for the principal series, the twisted Steinberg representations and the depth-zero supercuspidal representations. It compares the L-factors and the ε-factors of the pairs with characters of conductor exponent at most 2, in exact cyclotomic arithmetic. `lgl lab spec` prints the spec of each experiment.

## v0.1.2 (2026-11-26)

### Added

- The evaluation sets of spec 10.9 in `eval/`: 200 tasks with known answers in `eval/tasks/` (prove tasks from Mathlib NumberTheory and RepresentationTheory and from the project, state tasks from the NYLL port, and repair tasks from the renames of the NYLL port), and 50 statements with reviewed Lean and 50 wrong statements in `eval/statements/`.
- `lgl eval list`, `check`, `gen`, `run`, `compare`, `key` and `decl`. `run` scores the answers file of a runner and writes the pass rate and the cost of each set to `eval/results/`. `compare` exits with 1 when a pass rate drops by more than 5 points. lgl calls no model.
- The CI smoke job runs `lgl eval check`.

### Fixed

- The smoke job expects code 2 from `lgl agent` with no subcommand, because all the commands are written.
- Two findings of the new golangci-lint: G702 of gosec in `lgl agent run` and QF1001 of staticcheck in `lgl port check`.

## v0.1.1 (2026-11-25)

### Added

- `lgl claim add`, `list`, `show` and `plan` for the claim records of route R1 in `claims/`. `add` writes a fresh record with a stable key, the source and the date, and it fetches nothing. `plan` checks the record and prints each part with its statement, its cost and whether it is self-contained.
- `lgl check` checks the claim records: the key, the source, the dates, the statuses, the parts and the letters, and two records with the same key or the same source.
- `lgl queue`, which ranks the nodes that a role can act on with the formula of spec 10.4 and prints the reason for each rank. When `blueprint/routes.yaml` or `blueprint/milestones.yaml` is missing, it writes the file with the default weights.
- `lgl agent run`, which builds the prompt of a role for the top item of its queue or for a given node, and writes a run record with the status `planned` to `records/runs/`. It starts the command of `LGL_RUNNER` only when the budget of `agent.yaml` is more than zero, and it records the exit status and the output. With no runner the run stops after the record.
- `lgl agent report`, which writes the weekly report `reports/<week>.md` of spec 10.8 from the run records, the ledger, the lab runs, the claim records and the map, and a status snapshot `records/status/<week>.json` for the next report.

### Changed

- `lgl claim`, `lgl queue` and `lgl agent` are written, so they no longer exit with 3. `lgl agent` with no subcommand is a usage error.

## v0.1.0 (2026-11-25)

### Added

- `lgl fidelity list`, `show`, `set` and `run`, which keep `records/fidelity.json`. F1 is computed from `records/lean.json`, and `pass` is computed from the class of the statement and its checks (spec 06.3).
- `lgl check` reports an entry of `records/fidelity.json` that says `pass` but does not have the checks of its class.
- `lgl trust`, which writes `records/trust.json` with the answers Q0 to Q8 of spec 14. It checks the axioms of each declaration that a node names, reads the record of the kernel replay if there is one, and marks a row that it cannot check as `not run`.
- `lgl paper add`, `list` and `show` for the paper records of spec 07.13. `add` checks the new record as `lgl check` does, and it does not overwrite a file.
- `lgl lab list`, `run`, `spec` and `show` for the computation lab of spec 13. A run writes `lab/runs/<id>/` with `record.json` in the form that the site reads, `summary.md` and the outputs.
- A small version of experiment E1 in Go: it checks the norm groups of the quadratic and cyclotomic extensions of Q_p for p <= 7 against the Artin map of Lubin-Tate theory. It uses no container and no network, and it writes no Lean certificate.

## v0.0.2 (2026-11-25)

### Added

- `lgl port check`, which checks the licence header and the provenance line of each ported Lean file against `manifests/ported.yaml` of langlands-lean.
- The status `faithful` of spec 08.3, for a statement that passed the fidelity checks of its class in `records/fidelity.json`.
- The paper page shows the reader, the Lean status, the other formalizations and the results that the project needs.

### Fixed

- `lgl check` reads the paper records in the schema of spec 07.13: `id`, `read`, `results`, `lean_elsewhere`, `lean_status`, `code_licence` and `handbook`. Before this change it read an older schema and rejected the records of tamnd/langlands, so the deploy workflow failed.

## v0.0.1 (2026-11-25)

### Added

- `lgl check`, `lgl status`, `lgl index` and `lgl site`, adapted from singularity-agent for the map of tamnd/langlands.
- The tiers T0 to T10 of the corpus, the node kinds `check` and `claim`, and the status `claimed` for a fresh claim of route R1.
- The site pages of the program, the map, the statements, the trust report and the lab.

### Changed

- The tier titles follow spec 07, and the default trust questions Q0 to Q8 follow spec 14.
- The smoke test for exit code 3 uses `lgl agent`.

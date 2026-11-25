# langlands-agent

The toolchain of [langlands](https://github.com/tamnd/langlands), the public project that states the Langlands program in Lean 4, proves the known parts and works on the open parts. The command is `lgl`. It checks the map of the content repository, gives the status of each node from the record of the Lean build, and builds the site [langlands.tamnd.com](https://langlands.tamnd.com).

The general conjectures of the Langlands program are open. This toolchain does not decide that a result is proved. The Lean kernel decides that, and `lgl` only reads the record of the build.

## Install

You need Go 1.27 or later.

```sh
go install github.com/tamnd/langlands-agent/cmd/lgl@latest
```

Each release on GitHub also has binaries for Linux, macOS and Windows, with checksums and a build provenance attestation.

## Use

Run the commands in a clone of `tamnd/langlands`.

```sh
lgl check                      # check the map: tags, links, papers, routes and Lean names
lgl status                     # print the status of each node
lgl status -only stated        # print only the stated nodes
lgl index -lean lean.json      # copy the record of the Lean build to records/lean.json
lgl site build -content . -out public
lgl site serve -dir public     # serve the site at http://127.0.0.1:8080/
```

The exit codes are 0 for success, 1 for a failed check, 2 for a usage error and 3 for a command that is not written yet. The help text gives the milestone of each command that is not written yet: `trust`, `fidelity`, `port`, `paper` and `lab` come in S1, and `claim`, `queue` and `agent` come in S2.

## The status rules

The status of a node comes from `records/lean.json` and from the nodes that it uses. The rules are in `blueprint/blueprint.go`.

| Status | When |
|---|---|
| unstated | The node has no Lean statement in the build. |
| stated | The statement builds, but the proof uses `sorry` or an axiom that is not standard. |
| defined | A definition that builds with no `sorry`. |
| proved | The proof uses only `propext`, `Classical.choice` and `Quot.sound`, and each node that it uses is proved or defined. |
| proved-mod | The proof is clean but it takes named hypotheses, or a node that it uses is not proved. |
| open | A conjecture. No proof is claimed. |
| claimed | A fresh claim of route R1. It is never a hypothesis of another node. |
| refuted | The negation is proved. |

## The site

The site uses the style and the math renderer of [Math 2.0](https://github.com/tamnd/math2-agent). The math is rendered to MathML at build time, so a page shows its math with JavaScript off. Each page has a Markdown copy next to it, and `/api/` has the map, the papers and the trust report as JSON. The page `/lab/` lists the runs of `lab/runs/`, and each run has a page with its summary and its files.

## Develop

```sh
make check      # gofmt, vet, the tests with the race detector, and golangci-lint
```

Read [CONTRIBUTING.md](CONTRIBUTING.md) before you open a pull request.

## Licence

MIT. See [LICENSE](LICENSE). The files in `site/assets/` come from Math 2.0. See [site/assets/UPSTREAM.md](site/assets/UPSTREAM.md) for their sources and licences.

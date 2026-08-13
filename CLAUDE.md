# dusk-plugin-sdk

The plugin contract for [Dusk](https://github.com/NerdsWhoFish/dusk).
**The `.proto` is the contract**; the Go SDK is a convenience layer on top of it.

## Before you change anything

The decisions governing this repo live in the Dusk repo, not here:

- [ADR-0002](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0002-plugin-protocol.md): why subprocesses, why two transports, why not `hashicorp/go-plugin`
- [ADR-0007](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0007-entity-schema.md): why kinds are open strings and why there is an attributes escape hatch
- [ADR-0015](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0015-plugin-actions-and-events.md): actions, classification, and why events are not notes
- [ADR-0016](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0016-plugin-sdk-repo.md): why this repo is separate
- [ADR-0017](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0017-engineering-policy.md): the engineering policy, including testing and the cgo rule

New decisions about the contract are recorded as ADRs **in the Dusk repo**, and its `adr/README.md` index is updated in the same change.

## This is a published contract

Treat every proto change as breaking until proven otherwise.

- `make breaking` checks the branch against `main`, and CI runs it on every pull request.
- `make lint` runs `buf lint`. Its rules are not style preferences: reusing one request type across RPCs means you cannot evolve one without affecting the others.
- **Generated code is committed** so consumers can `go get` without buf. CI fails if `gen/` drifts from the proto, so run `make generate` and commit the result.
- Status is `v1alpha1`. No stability is promised until the Kubernetes, Flux, and GitHub ingesters have shipped.

## Non-negotiables

- **`ActionClass` is a closed enum; `kind` and relation `type` are not.** Closed where the meaning must be identical everywhere, open where the taxonomy must grow without a release.
- **`partial` must survive the wire.** It is how a plugin says "I could not fully look", and losing it makes Dusk delete real entities on a transient failure.
- **Both field namings must parse.** Shell scripts write `schema_version`, protojson emits `schemaVersion`. Breaking either breaks Tier 1 for everyone hand-writing JSON.
- **The README example is a test.** It promises a shell script is enough; if its output stops validating, that promise is false.
- **No cgo.** `make nocgo` enforces it.

## Working here

```bash
make check   # lint + nocgo + test, what CI runs
make test    # go test -race ./...
```

Tests assert observable results, are table-driven by default, and use the standard library only.
Rules an ADR calls load-bearing get a test named after them: `TestADR0011_PartialFlagSurvivesRoundTrip`.

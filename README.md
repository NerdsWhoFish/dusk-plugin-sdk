# dusk-plugin-sdk

The plugin contract for [Dusk](https://github.com/FetchHQ/dusk).

**The `.proto` is the contract.** The Go SDK is a convenience layer on top of it.
If you are writing a plugin in Python, Rust, TypeScript, or a shell script, you need only `proto/`.

> Status: **`v1alpha1`**. No stability is promised until the three in-house ingesters (Kubernetes, Flux, GitHub) have shipped.

## Two tiers, one schema

Dusk plugins come in two shapes, and they share a single set of message definitions.

**Tier 1, ingesters.** Dusk executes a binary, the binary writes an `IngestBatch` as **protojson** on stdout, and exits. No gRPC, no codegen, no build step. A plugin can be a shell script.

**Tier 2, interactive plugins.** The identical messages over gRPC on a unix socket Dusk provides. Needed only for long-lived work or for exposing actions.

A plugin can graduate from Tier 1 to Tier 2 without changing its data model.

## A complete Tier 1 plugin

```bash
#!/usr/bin/env bash
kubectl get deploy -A -o json | jq '{
  schema_version: "v1alpha1",
  entities: [.items[] | {
    ref: ("service:" + .metadata.namespace + "/" + .metadata.name),
    kind: "service",
    namespace: .metadata.namespace,
    name: .metadata.name,
    provenance: { source: "kubectl-sh", observed_at: (now | todate) }
  }]
}'
```

Test it the way you would test anything else:

```bash
./my-plugin | jq .
```

## Contract rules that are easy to get wrong

**Set `partial: true` if you could not fully enumerate.** Dusk will not treat absence as deletion when this is set. "I could not look" and "it is not there" must never be the same thing, or a timeout looks identical to a decommissioned service.

**`kind` and relation `type` are open vocabulary.** Do not invent one casually. Dusk gates new kinds behind `getKinds` and `mintKind` so the taxonomy does not fragment into `service`, `svc`, and `srvice`.

**Fill in `Provenance` on everything.** Dusk never infers where a claim came from.

**Observations do not overwrite declarations.** A `dusk.md` declares intent; your plugin observes reality. Divergence surfaces as drift, which is the point.

**Dry run is required for every action.** Returning `supported: false` is a valid answer. Dusk surfaces that at approval time, so a human knows a destructive action cannot be previewed.

## Layout

```text
proto/dusk/v1alpha1/
├── entity.proto   Entity, Relation, Note, Observation, Provenance, IngestBatch
├── plugin.proto   Plugin service, action declarations, invocation
└── event.proto    Event, emitted on every action invocation
```

## Development

Requires [buf](https://buf.build/docs/installation).

```bash
make lint       # lint the proto
make generate   # generate Go into gen/
make breaking   # check this branch against main
```

## Design

The reasoning behind this contract lives in the Dusk repo:

- [ADR-0002](https://github.com/FetchHQ/dusk/blob/main/adr/0002-plugin-protocol.md): why subprocesses, why two transports, why not `hashicorp/go-plugin`
- [ADR-0007](https://github.com/FetchHQ/dusk/blob/main/adr/0007-entity-schema.md): why open kinds and an attributes escape hatch
- [ADR-0015](https://github.com/FetchHQ/dusk/blob/main/adr/0015-plugin-actions-and-events.md): actions, classification, and why events are not notes
- [ADR-0016](https://github.com/FetchHQ/dusk/blob/main/adr/0016-plugin-sdk-repo.md): why this repo is separate

## License

Apache 2.0. Contributions under the DCO.

The `.proto` and the entity schema are explicitly covered, so there is no license ambiguity about what you are compiling against.

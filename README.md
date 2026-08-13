# dusk-plugin-sdk

The plugin contract for [Dusk](https://github.com/NerdsWhoFish/dusk).

**The `.proto` is the contract.** The Go packages are a convenience layer on top of it.
If you are writing a plugin in Python, Rust or TypeScript, you need only `proto/`.

> Status: **`v1alpha1`**. No stability is promised until the in-house plugins have shipped.

## One transport

A plugin is a binary Dusk execs, serving `PluginService` over gRPC on a unix socket Dusk provides.
There is no second way in ([ADR-0039](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0039-one-plugin-transport.md)).

Every gRPC implementation accepts a `unix:` target, which is why the socket is a path rather than an inherited descriptor: a transport only Go implements comfortably would quietly undo the language-agnostic promise this protocol exists to keep ([ADR-0044](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0044-plugins-keep-the-socket-directory.md)).

**Something that only wants an entity in the catalog does not need a plugin at all.** Write a `dusk.md`, which needs no binary, no protocol and no permission.

## What a plugin declares

`Describe` is answered once at start, and everything downstream is built from it.

| Field | What it decides |
| --- | --- |
| `emits_kinds` | What this plugin may put in the catalog |
| `config_fields` | The form Dusk renders. `sensitive` is a first-class field, not an annotation, because an author who forgets it leaks a credential |
| `actions` | What can be done, each with a class, a JSON Schema, and the read that satisfies read-before-write |
| `ui` | Views. Either **declared**, and Dusk renders them, or an **element**, and the plugin ships JavaScript |
| `budget` | Which config fields identify the upstream system, so two configurations pointed at one server queue rather than each assuming the whole quota |

Check it before Dusk does:

```go
if result := conformance.ValidateDescribe(described); !result.OK() {
    log.Fatal(result.Error())
}
```

## Serving one

The Go SDK runs the process for you: it binds the socket, requires the host's token on every call, and shuts down politely.

```go
func main() {
    err := plugin.Run(&myPlugin{}, plugin.Options{Version: version})
    if err != nil {
        slog.Error("my-plugin", "error", err)
        os.Exit(1)
    }
}
```

The token matters. Every socket shares one directory and one user, so any plugin can dial another's; requiring the token makes that useless and keeps composition going through Dusk, where it is approved and recorded.

## Contract rules that are easy to get wrong

**Set `partial: true` if you could not fully enumerate.** Dusk will not treat absence as deletion when this is set. "I could not look" and "it is not there" must never be the same thing, or a timeout looks identical to a decommissioned service.

**`kind` and relation `type` are open vocabulary.** Do not invent one casually. Dusk gates new kinds so the taxonomy does not fragment into `service`, `svc`, and `srvice`.

**Fill in `Provenance` on everything.** Dusk never infers where a claim came from.

**Observations do not overwrite declarations.** A `dusk.md` declares intent; your plugin observes reality. Divergence surfaces as drift, which is the point.

**Dry run is required for every action.** Returning `supported: false` is a valid answer. Dusk surfaces that at approval time, so a human knows a destructive action cannot be previewed.

**An invocation may only return follow-on steps its descriptor declared.** Otherwise approving a chain would mean approving whatever the plugin decided to append once it was running.

## Layout

```text
proto/dusk/v1alpha1/
├── entity.proto   Entity, Relation, Note, Observation, Provenance, IngestBatch
├── plugin.proto   Plugin service, action declarations, invocation, views, budget
└── event.proto    Event, emitted on every action invocation

conformance/       Validate a batch, a config form, or a whole Describe
plugin/            Run a plugin: socket, token, graceful shutdown
```

## Development

Requires [buf](https://buf.build/docs/installation).

```bash
make check      # lint + nocgo + test, what CI runs
make generate   # generate Go into gen/
make breaking   # check this branch against main
```

## Design

The reasoning behind this contract lives in the Dusk repo:

- [ADR-0002](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0002-plugin-protocol.md): why subprocesses, why not `hashicorp/go-plugin`
- [ADR-0007](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0007-entity-schema.md): why open kinds and an attributes escape hatch
- [ADR-0015](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0015-plugin-actions-and-events.md): actions, classification, and why events are not notes
- [ADR-0016](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0016-plugin-sdk-repo.md): why this repo is separate
- [ADR-0020](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0020-plugin-ui.md): views in tiers, and why a plugin ships a custom element rather than a React component
- [ADR-0039](https://github.com/NerdsWhoFish/dusk/blob/main/adr/0039-one-plugin-transport.md): one transport, and what it cost

## License

Apache 2.0. Contributions under the DCO.

The `.proto` and the entity schema are explicitly covered, so there is no license ambiguity about what you are compiling against.

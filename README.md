# jsonot

[![PR Check](https://github.com/edocevol/jsonot/actions/workflows/pr-check.yml/badge.svg)](https://github.com/edocevol/jsonot/actions/workflows/pr-check.yml)
[![Release](https://img.shields.io/github/v/release/edocevol/jsonot)](https://github.com/edocevol/jsonot/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/edocevol/jsonot)](./go.mod)
[![License](https://img.shields.io/github/license/edocevol/jsonot)](./LICENSE)

[简体中文](./README.zh-CN.md)

`jsonot` is a Go library for JSON Operational Transformation (OT). It helps you apply JSON OT operations, transform concurrent edits, build collaboration backends, and generate diff / revert operations for versioned JSON documents.

The project is inspired by [ylgrgyq/json0-rs](https://github.com/ylgrgyq/json0-rs).

## What is jsonot?

Use `jsonot` when you need a Go-native JSON OT engine for scenarios such as:

- collaborative editing in Go
- WebSocket collaborative editor backends
- ShareDB-style backends in Go
- JSON diff / revert and version restore flows
- server-authoritative merge pipelines for rich text or structured JSON documents

If you are searching for a **Go OT library**, **JSON OT for Go**, or a **ShareDB alternative in Go**, this repository is the main entry point.

## Who is it for?

`jsonot` is a good fit when you need:

- a backend OT core for collaborative editors
- server-side rebasing with `Transform`
- document restore and rollback with `Diff`
- a small Go package instead of a full collaboration platform
- a Go package that you can embed inside your own WebSocket or HTTP services

## Start here

Choose the entry point that matches your goal:

- **I want to use the library directly** → start with [Quick start](#quick-start) and [What problems does jsonot solve?](#what-problems-does-jsonot-solve)
- **I want a collaborative editing demo** → see [WebSocket demo](./examples/websocket/README.md) and [BlockNote demo](./examples/blocknote-collab/README.md)
- **I want a ShareDB-style backend** → see [`jsonot/sharedb`](./sharedb/README.md)

## What problems does jsonot solve?

### 1. Collaborative editing in Go

`jsonot` provides the OT core needed to rebase concurrent operations and keep a server-authoritative document consistent.

### 2. JSON diff, revert, and version restore

`Diff` can generate JSON OT that transforms one version of a document into another, which is useful for rollback, restore, comparison, and audit flows.

### 3. ShareDB-style backend building blocks

The `sharedb` package shows how to layer snapshots, versions, submit, and subscribe APIs on top of the core OT engine.

## Features

- Apply JSON OT operations to objects, arrays, numbers, and text values
- Transform concurrent operations with `Transform`
- Compose multiple operations into a single operation
- Generate diff / revert operations from two JSON documents with `Diff`
- Build operations with the provided builders
- Switch between the default Sonic backend and the AJSON backend
- Use the lightweight `sharedb` package for ShareDB-style backend flows
- Explore runnable collaboration demos for WebSocket text editing and BlockNote documents

## Capability matrix

| Capability | jsonot | Where to start |
| --- | --- | --- |
| Apply JSON OT to documents | Yes | [`Apply`](./apply.go), [Quick start](#quick-start) |
| Rebase concurrent operations | Yes | [`Transform`](./transform.go), [Transforming concurrent operations](#transforming-concurrent-operations) |
| Compose multiple ops | Yes | [`Compose`](./jsonot.go) |
| Generate diff / revert operations | Yes | [Generating revert operations from two versions](#generating-revert-operations-from-two-versions) |
| ShareDB-style backend abstraction | Yes | [`jsonot/sharedb`](./sharedb/README.md) |
| WebSocket collaboration demo | Yes | [`examples/websocket`](./examples/websocket/README.md) |
| Rich text collaboration demo | Yes | [`examples/blocknote-collab`](./examples/blocknote-collab/README.md) |

## Scenario matrix

| Scenario | Recommended starting point |
| --- | --- |
| Build a collaborative editor backend in Go | [WebSocket demo](./examples/websocket/README.md) |
| Build a rich text collaboration backend | [BlockNote demo](./examples/blocknote-collab/README.md) |
| Build a ShareDB-like backend in Go | [`jsonot/sharedb`](./sharedb/README.md) |
| Restore an older JSON version | [`Diff`](#generating-revert-operations-from-two-versions) |
| Rebase concurrent client operations on the server | [`Transform`](#transforming-concurrent-operations) |

## Quick start

```go
package main

import (
"context"
"fmt"

"github.com/edocevol/jsonot"
)

func main() {
ot := jsonot.NewJSONOperationTransformer()

doc, _ := jsonot.UnmarshalValue([]byte(`{
"name": "json0",
"hobbies": ["reading", "coding", "music"],
"info": {"email": "example@mail.qq.com"}
}`))

rawOps, _ := jsonot.UnmarshalValue([]byte(`[
{"p": ["hobbies", "2"], "ld": "music"},
{"p": ["hobbies", "3"], "li": "movie"},
{"p": ["info", "email"], "od": "example@mail.qq.com"}
]`))

components := ot.OperationComponentsFromValue(rawOps)
op := jsonot.NewOperation(components.MustGet())

result := ot.Apply(context.Background(), doc, op)
fmt.Println(string(result.MustGet().RawMessage()))
}
```

## Operation format

Operation components are represented as JSON objects. The `p` field is the target path.

### Built-in actions

- `li`: list insert
- `ld`: list delete
- `lm`: list move
- `oi`: object insert
- `od`: object delete
- `na`: number add subtype
- `t` + `o`: custom subtype name and operand

Examples:

```json
{"p": ["todos", "1"], "li": {"title": "review"}}
{"p": ["profile", "name"], "oi": "jsonot"}
{"p": ["counter"], "na": 1}
{"p": ["content"], "t": "text", "o": {"p": 5, "i": " world"}}
```

## Building operations in Go

```go
ot := jsonot.NewJSONOperationTransformer()
factory := ot.OperationFactory()

component := factory.ObjectOperationBuilder(
jsonot.NewPathFromKeys([]string{"profile", "name"}),
).Insert(jsonot.ValueFromPrimitive("jsonot")).Build()

op := jsonot.NewOperation([]*jsonot.OperationComponent{component.MustGet()})
```

## Transforming concurrent operations

Use `Transform` when two operations were created from the same base document and need to be rebased against each other.

```go
leftPrime, rightPrime, err := ot.Transform(context.Background(), leftOp, rightOp)
```

After that, apply `leftPrime` after `rightOp`, or `rightPrime` after `leftOp`.

## Generating revert operations from two versions

Use `Diff` to generate JSON OT that transforms one document into another.

```go
current, _ := jsonot.UnmarshalValue([]byte(`{"version":2,"items":[1,2,3]}`))
previous, _ := jsonot.UnmarshalValue([]byte(`{"version":1,"items":[1,3]}`))

revertOp := ot.Diff(context.Background(), current, previous)
restored := ot.Apply(context.Background(), current, revertOp.MustGet())
```

This is useful for version comparison and restore flows. When a structure changes too much, `Diff` falls back to replacing that subtree, including the root document when needed.

## Value backends

The library uses Sonic by default.

```go
jsonot.UseSonic()
jsonot.UseAJSON()
```

Switch the backend before creating or parsing values so all `Value` instances come from the same implementation.

## Demos and guides

- [What is jsonot?](./docs/what-is-jsonot.md)
- [How to build collaborative editing in Go with JSON OT](./docs/go-json-ot-collaboration.md)
- [How to use jsonot for JSON diff / revert](./docs/json-diff-revert.md)
- [How to build a ShareDB-style backend in Go](./docs/sharedb-style-backend.md)
- [WebSocket collaboration demo](./examples/websocket/README.md)
- [BlockNote collaboration demo](./examples/blocknote-collab/README.md)
- [`jsonot/sharedb`](./sharedb/README.md)

## How is jsonot different?

### jsonot vs CRDT

If you prefer a server-authoritative merge flow and explicit rebasing with `Transform`, `jsonot` gives you an OT-centric path instead of a CRDT data model.

### jsonot vs ShareDB

ShareDB is a full collaboration system. `jsonot` focuses on a Go-native OT engine plus lightweight backend primitives that you can embed in your own services.

### jsonot vs plain JSON diff

Plain diff can tell you what changed. `jsonot` also helps you transform and rebase concurrent operations before applying them.

## FAQ

### Is jsonot a Go library for collaborative editing?

Yes. The core package handles OT transforms and applies operations, while the examples show how to build collaborative editing servers in Go.

### Can jsonot be used as a ShareDB alternative in Go?

Yes, when you want Go-native OT primitives and lightweight backend abstractions instead of the full ShareDB stack. Start with [`jsonot/sharedb`](./sharedb/README.md).

### Can jsonot generate rollback operations from two JSON versions?

Yes. Use `Diff` to derive an operation that transforms the current document into an earlier version.

### Can I use jsonot with rich text editors?

Yes. The [BlockNote demo](./examples/blocknote-collab/README.md) shows a rich text collaboration backend that sends snapshots and merges them with `Diff`, `Transform`, and `Apply`.

### What is the smallest runnable path?

1. `go get github.com/edocevol/jsonot`
2. copy the quick-start example from this README
3. run `go test ./...`
4. open one of the demos when you need an end-to-end collaboration flow

## Trust signals

- CI runs `go test ./...` in [PR Check](./.github/workflows/pr-check.yml)
- tag pushes validate tests before publishing a GitHub release in [Release Tag](./.github/workflows/release-tag.yml)
- the repository includes runnable demos and a dedicated `sharedb` package

## Testing

```bash
go test ./...
```

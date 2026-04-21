# What is jsonot?

`jsonot` is a Go library for JSON Operational Transformation (OT).

It is designed for developers who need a Go-native OT engine for JSON documents, collaboration backends, version restore flows, or ShareDB-style server logic.

## Core positioning

Use `jsonot` when you need:

- JSON OT in Go
- collaborative editing in Go
- server-side rebasing of concurrent operations
- JSON diff / revert operations for version restore
- a ShareDB-style backend foundation in Go

## What jsonot gives you

- `Apply` to apply OT operations to JSON documents
- `Transform` to rebase concurrent operations
- `Compose` to combine multiple operations
- `Diff` to derive restore operations from two JSON versions
- `sharedb` primitives for snapshots, versions, submit, and subscribe
- runnable demos for WebSocket text collaboration and BlockNote rich text collaboration

## Typical use cases

### Collaborative editing backend

Use `Transform` and `Apply` on the server when multiple clients edit the same document concurrently.

### Version comparison and restore

Use `Diff` when you want to generate an operation that restores one document version from another.

### ShareDB-style backend in Go

Use `jsonot/sharedb` when you want snapshots, versions, submit, and subscription concepts in a smaller Go-native package.

## Which repository page should I read next?

- want to use the core library? → [README](../README.md)
- want to build collaborative editing in Go? → [go-json-ot-collaboration](./go-json-ot-collaboration.md)
- want JSON diff / revert guidance? → [json-diff-revert](./json-diff-revert.md)
- want a ShareDB-style backend? → [sharedb-style-backend](./sharedb-style-backend.md)

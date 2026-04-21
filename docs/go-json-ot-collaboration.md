# How to build collaborative editing in Go with JSON OT

This guide explains how `jsonot` fits into a collaborative editing backend in Go.

## When should you use this approach?

Use this approach when:

- the server should keep the authoritative document
- clients submit operations or snapshots against a known version
- concurrent client edits need rebasing before they are applied
- you want a Go-native collaboration backend instead of a larger external platform

## Core server loop

A typical collaboration loop with `jsonot` looks like this:

1. load the client's base version
2. if the server already has newer operations, transform the incoming operation against them
3. apply the transformed operation to the current document
4. persist the new version
5. broadcast the latest document or operation to other clients

## Two implementation paths in this repository

### Path 1: operation-based text collaboration

See the [WebSocket demo](../examples/websocket/README.md) for a minimal browser-to-server OT flow.

### Path 2: snapshot-based rich text collaboration

See the [BlockNote demo](../examples/blocknote-collab/README.md) for a snapshot-to-OT flow using `Diff`, `Transform`, and `Apply`.

## Why OT here instead of plain diff?

Plain diff can describe a change between versions, but collaboration backends also need to reconcile concurrent edits. `jsonot` provides `Transform` for that extra step.

## Why OT here instead of CRDT?

Choose this route when you prefer explicit server-side rebasing and a server-authoritative merge flow over a CRDT data model.

## Recommended next steps

- start from the [README](../README.md) quick start
- run the [WebSocket demo](../examples/websocket/README.md)
- inspect [`jsonot/sharedb`](../sharedb/README.md) if you want backend abstractions around versions and subscriptions

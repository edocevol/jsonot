# How to build a ShareDB-style backend in Go with jsonot

If you want ShareDB-style ideas in a Go codebase, `jsonot` provides a small starting point.

## What "ShareDB-style" means here

In this repository, it means a backend built around:

- versioned document snapshots
- submit-by-version semantics
- server-side OT rebase for concurrent edits
- update subscriptions for connected clients

## Where to start

Start with [`jsonot/sharedb`](../sharedb/README.md).

That package shows how to:

- create a document
- keep a version number
- accept operations against an older version
- transform them against missing history
- notify subscribers when a commit succeeds

## Why use this instead of a full ShareDB stack?

Use this route when you want:

- Go-native code and APIs
- a small embeddable abstraction
- direct control over persistence, auth, rooms, and transport
- the ability to pair the backend with your own WebSocket or HTTP layer

## Related examples

- [WebSocket collaboration demo](../examples/websocket/README.md)
- [BlockNote collaboration demo](../examples/blocknote-collab/README.md)
- [Root README](../README.md)

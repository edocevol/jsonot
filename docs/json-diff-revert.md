# How to use jsonot for JSON diff / revert

`jsonot` can generate JSON OT that transforms one JSON document version into another.

## When is this useful?

Use `Diff` when you need:

- version restore or rollback
- document comparison flows
- audit and change recovery tooling
- a JSON-aware way to describe state transitions with OT

## Basic pattern

1. load the current document
2. load the target document you want to restore or compare against
3. call `Diff(current, target)`
4. apply the resulting operation to the current document

## Example

```go
current, _ := jsonot.UnmarshalValue([]byte(`{"version":2,"items":[1,2,3]}`))
previous, _ := jsonot.UnmarshalValue([]byte(`{"version":1,"items":[1,3]}`))

revertOp := ot.Diff(context.Background(), current, previous)
restored := ot.Apply(context.Background(), current, revertOp.MustGet())
```

## What happens when the structure changes a lot?

When a subtree changes too much, `Diff` falls back to replacing that subtree. If needed, it can replace the entire root document.

## How is this different from plain JSON diff?

The output is JSON OT that can participate in the same operation pipeline as other `jsonot` operations. That makes it more useful when your system already works with OT.

## Recommended next steps

- read the [root README](../README.md)
- inspect the [BlockNote demo](../examples/blocknote-collab/README.md) for a snapshot-to-OT collaboration pattern

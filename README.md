# jsonot

[简体中文](./README.zh-CN.md)

`jsonot` is a Go library for JSON Operational Transformation (OT). It can parse JSON OT operations, apply them to JSON documents, and transform concurrent operations so they can be merged safely.

The project is inspired by [ylgrgyq/json0-rs](https://github.com/ylgrgyq/json0-rs).

## Features

- Apply JSON OT operations to objects, arrays, numbers, and text values
- Transform concurrent operations with `Transform`
- Compose multiple operations into a single operation
- Generate diff/revert operations from two JSON documents with `Diff`
- Build operations with the provided operation builders
- Switch between the default Sonic backend and the AJSON backend

## Installation

```bash
go get github.com/edocevol/jsonot
```

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

## Testing

```bash
go test ./...
```

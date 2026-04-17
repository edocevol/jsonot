# jsonot

[English](./README.md)

`jsonot` 是一个 Go 语言的 JSON Operational Transformation（OT）库，用于解析 JSON OT 操作、将操作应用到 JSON 文档上，以及对并发操作进行转换。

项目灵感来自 [ylgrgyq/json0-rs](https://github.com/ylgrgyq/json0-rs)。

## 功能特性

- 支持对对象、数组、数字和文本值应用 JSON OT 操作
- 支持通过 `Transform` 处理并发操作
- 支持将多个操作组合为一个操作
- 支持通过 `Diff` 基于两个 JSON 文档生成 diff / revert 操作
- 提供 operation builder 以便在 Go 代码中构建操作
- 支持在默认的 Sonic 后端和 AJSON 后端之间切换

## 安装

```bash
go get github.com/edocevol/jsonot
```

## 快速开始

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

## 操作格式

操作组件使用 JSON 对象表示，其中 `p` 表示目标路径。

### 内置操作字段

- `li`：数组插入
- `ld`：数组删除
- `lm`：数组移动
- `oi`：对象插入
- `od`：对象删除
- `na`：数字加法子类型
- `t` + `o`：自定义子类型名称与操作数

示例：

```json
{"p": ["todos", "1"], "li": {"title": "review"}}
{"p": ["profile", "name"], "oi": "jsonot"}
{"p": ["counter"], "na": 1}
{"p": ["content"], "t": "text", "o": {"p": 5, "i": " world"}}
```

## 在 Go 中构建操作

```go
ot := jsonot.NewJSONOperationTransformer()
factory := ot.OperationFactory()

component := factory.ObjectOperationBuilder(
	jsonot.NewPathFromKeys([]string{"profile", "name"}),
).Insert(jsonot.ValueFromPrimitive("jsonot")).Build()

op := jsonot.NewOperation([]*jsonot.OperationComponent{component.MustGet()})
```

## 转换并发操作

当两个操作基于同一份初始文档生成，并且需要互相重放时，可以使用 `Transform`：

```go
leftPrime, rightPrime, err := ot.Transform(context.Background(), leftOp, rightOp)
```

随后可以在应用 `rightOp` 后应用 `leftPrime`，或者在应用 `leftOp` 后应用 `rightPrime`。

## 基于两个版本生成恢复操作

可以使用 `Diff` 生成将一个文档恢复到另一个版本的 JSON OT。

```go
current, _ := jsonot.UnmarshalValue([]byte(`{"version":2,"items":[1,2,3]}`))
previous, _ := jsonot.UnmarshalValue([]byte(`{"version":1,"items":[1,3]}`))

revertOp := ot.Diff(context.Background(), current, previous)
restored := ot.Apply(context.Background(), current, revertOp.MustGet())
```

这个能力适合做版本对比和恢复。对于变更较大的结构，`Diff` 会直接替换对应子树；必要时也支持替换整个根文档。

## Value 后端

库默认使用 Sonic。

```go
jsonot.UseSonic()
jsonot.UseAJSON()
```

建议在创建或解析 `Value` 之前先确定后端实现，以确保参与操作的 `Value` 类型一致。

## 测试

```bash
go test ./...
```

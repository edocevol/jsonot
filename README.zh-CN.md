# jsonot

[![PR Check](https://github.com/edocevol/jsonot/actions/workflows/pr-check.yml/badge.svg)](https://github.com/edocevol/jsonot/actions/workflows/pr-check.yml)
[![Release](https://img.shields.io/github/v/release/edocevol/jsonot)](https://github.com/edocevol/jsonot/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/edocevol/jsonot)](./go.mod)
[![License](https://img.shields.io/github/license/edocevol/jsonot)](./LICENSE)

[English](./README.md)

`jsonot` 是一个 Go 语言的 JSON Operational Transformation（OT）库，适合用来解析 JSON OT 操作、应用文档变更、处理并发操作转换，以及构建协同编辑和版本恢复能力。

项目灵感来自 [ylgrgyq/json0-rs](https://github.com/ylgrgyq/json0-rs)。

## jsonot 是什么？

如果你在找下面这些能力，`jsonot` 就是这个仓库的核心定位：

- Go OT 库
- Go 语言的 JSON OT 引擎
- Go 协同编辑后端核心
- ShareDB 风格后端的 Go 实现入口
- JSON diff / revert / 回滚恢复能力
- 富文本或结构化 JSON 的服务端权威合并

## 适合谁？

当你需要以下能力时，`jsonot` 会比较合适：

- 在 Go 服务端处理协同编辑并发合并
- 用 `Transform` 对客户端并发操作做 rebase
- 用 `Diff` 做文档版本对比、恢复和回滚
- 想要一个可嵌入自身 WebSocket / HTTP 服务的轻量 OT 内核
- 想在 Go 生态里实现 ShareDB 类似能力，而不是直接依赖完整平台

## 从哪里开始？

根据目标选择入口：

- **我想直接用库** → 先看[快速开始](#快速开始)和[jsonot 解决什么问题？](#jsonot-解决什么问题)
- **我想跑协同编辑 demo** → 看 [WebSocket 示例](./examples/websocket/README.md) 和 [BlockNote 示例](./examples/blocknote-collab/README.md)
- **我想做 ShareDB 风格后端** → 看 [`jsonot/sharedb`](./sharedb/README.md)

## jsonot 解决什么问题？

### 1. 用 Go 做协同编辑

`jsonot` 提供了协同编辑服务端最核心的 OT 能力：接收操作、转换并发操作、按顺序应用到权威文档。

### 2. JSON diff / revert / 版本恢复

`Diff` 可以生成“从当前版本恢复到目标版本”的 JSON OT，适合做版本恢复、回滚、对比和审计场景。

### 3. 用 Go 搭 ShareDB 风格后端

`sharedb` 子包展示了如何在 OT 内核上叠加快照、版本、提交和订阅接口。

## 功能特性

- 支持对对象、数组、数字和文本值应用 JSON OT 操作
- 支持通过 `Transform` 处理并发操作
- 支持将多个操作组合为一个操作
- 支持通过 `Diff` 基于两个 JSON 文档生成 diff / revert 操作
- 提供 operation builder 以便在 Go 代码中构建操作
- 支持在默认的 Sonic 后端和 AJSON 后端之间切换
- 提供轻量级 `sharedb` 包，用于 ShareDB 风格后端流程
- 提供可运行的 WebSocket 文本协同和 BlockNote 富文本协同示例

## 能力矩阵

| 能力 | 是否支持 | 入口 |
| --- | --- | --- |
| 对 JSON 文档应用 OT | 支持 | [`Apply`](./apply.go)、[快速开始](#快速开始) |
| 并发操作转换 / rebase | 支持 | [`Transform`](./transform.go)、[转换并发操作](#转换并发操作) |
| 多操作合并 | 支持 | [`Compose`](./jsonot.go) |
| 生成 diff / revert / 回滚操作 | 支持 | [基于两个版本生成恢复操作](#基于两个版本生成恢复操作) |
| ShareDB 风格后端抽象 | 支持 | [`jsonot/sharedb`](./sharedb/README.md) |
| WebSocket 协同编辑 demo | 支持 | [`examples/websocket`](./examples/websocket/README.md) |
| 富文本协同 demo | 支持 | [`examples/blocknote-collab`](./examples/blocknote-collab/README.md) |

## 使用场景矩阵

| 场景 | 推荐入口 |
| --- | --- |
| 用 Go 实现协同编辑后端 | [WebSocket 示例](./examples/websocket/README.md) |
| 做富文本协同后端 | [BlockNote 示例](./examples/blocknote-collab/README.md) |
| 做 ShareDB 类似后端 | [`jsonot/sharedb`](./sharedb/README.md) |
| 恢复旧版本 JSON 文档 | [`Diff`](#基于两个版本生成恢复操作) |
| 在服务端 rebase 客户端并发操作 | [`Transform`](#转换并发操作) |

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

## 深入文档与专题内容

- [jsonot 是什么？](./docs/what-is-jsonot.md)
- [如何用 Go 做 JSON OT 协同编辑](./docs/go-json-ot-collaboration.md)
- [如何用 jsonot 做 JSON diff / revert / 回滚](./docs/json-diff-revert.md)
- [如何基于 jsonot 构建 ShareDB 风格后端](./docs/sharedb-style-backend.md)
- [WebSocket 协同编辑示例](./examples/websocket/README.md)
- [BlockNote 协同编辑示例](./examples/blocknote-collab/README.md)
- [`jsonot/sharedb`](./sharedb/README.md)

## jsonot 和其他方案有什么不同？

### jsonot vs CRDT

如果你更偏向服务端权威合并、显式 rebase 和操作转换流程，`jsonot` 提供的是 OT 路线，而不是 CRDT 数据模型路线。

### jsonot vs ShareDB

ShareDB 更像完整协同系统；`jsonot` 更聚焦于 Go 原生 OT 内核和可嵌入的轻量后端抽象。

### jsonot vs 纯 JSON diff

纯 diff 只能描述“发生了什么变化”；`jsonot` 还可以在应用前处理并发操作转换。

## FAQ

### jsonot 能用来做 Go 协同编辑吗？

可以。核心库负责 OT 转换与应用，示例展示了如何在 Go 中实现协同编辑服务端。

### jsonot 能作为 Go 版 ShareDB alternative 吗？

可以，尤其适合你想要 Go 原生 OT 能力和轻量后端抽象，而不是完整 ShareDB 技术栈时。建议从 [`jsonot/sharedb`](./sharedb/README.md) 开始。

### jsonot 能根据两个 JSON 版本生成回滚操作吗？

可以。使用 `Diff` 就能生成把当前文档恢复到旧版本的操作。

### jsonot 能配合富文本编辑器使用吗？

可以。[BlockNote 示例](./examples/blocknote-collab/README.md) 展示了快照上行、服务端用 `Diff`、`Transform`、`Apply` 合并的方式。

### 最小可运行接入路径是什么？

1. `go get github.com/edocevol/jsonot`
2. 复制本 README 里的快速开始示例
3. 运行 `go test ./...`
4. 需要端到端协同时，再进入 demo 或 `sharedb` 文档

## 可信度信号

- CI 在 [PR Check](./.github/workflows/pr-check.yml) 中执行 `go test ./...`
- 打 tag 发布前会在 [Release Tag](./.github/workflows/release-tag.yml) 中再次校验测试
- 仓库内自带可运行 demo 和独立的 `sharedb` 子包

## 测试

```bash
go test ./...
```

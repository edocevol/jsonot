# BlockNote 协同编辑 Demo（jsonot）

这个示例把 [BlockNote](https://github.com/TypeCellOS/BlockNote) 作为富文本 block 编辑器，
并使用 `jsonot` 在服务端完成 OT 并发合并。

## 能力说明

- BlockNote 富文本块编辑
- WebSocket 实时同步
- 客户端发送文档快照（`{ blocks: [...] }`）
- 服务端对快照执行：
  - `Diff(baseSnapshot, clientSnapshot)` 生成 OT
  - `Transform(clientOp, concurrentOps)` 处理并发
  - `Apply(currentDoc, transformedOp)` 更新权威文档
- 广播最新版本文档给其他客户端

## 目录

- `main.go`：Go WebSocket 协同后端
- `web/index.html`：React + BlockNote 前端（ESM CDN）
- `go.mod`：独立示例模块

## 运行

```bash
cd examples/blocknote-collab
go mod tidy
go run .
```

默认地址：`http://127.0.0.1:8080`

## 使用

1. 打开 `http://127.0.0.1:8080`
2. 再开一个浏览器窗口访问同一地址
3. 在两个窗口同时编辑，观察版本号与内容同步

## 协议

客户端发送：

```json
{
  "type": "sync",
  "version": 3,
  "document": {
    "blocks": [
      {"type": "paragraph", "content": "hello"}
    ]
  }
}
```

服务端消息：

- `init`：初始化客户端、文档和版本
- `ack`：确认当前客户端提交
- `update`：广播给其他客户端
- `error`：同步失败

## 注意事项

- 为了让 `Diff` 结果稳定，服务端要求 `document.blocks` 是非空数组。
- 该示例采用“快照上行，OT 内核合并”的模式，便于接入复杂编辑器。
- 生产环境建议补充：鉴权、房间隔离、持久化、断线重连与光标协同。

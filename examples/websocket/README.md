# WebSocket 协同编辑示例

这个示例在单独的 Go module 中，通过 WebSocket 把浏览器里的文本改动发送到服务端，再由服务端使用 `jsonot` 完成并发操作转换与文档更新。

## 结构

- `main.go`：HTTP + WebSocket 服务端
- `web/index.html`：浏览器前端页面
- `go.mod`：独立示例模块，使用 `replace` 指向仓库根目录的本地 `jsonot`

## 运行

```bash
cd examples/websocket
go run .
```

默认监听 `http://127.0.0.1:8080`。

## 体验方式

1. 在浏览器中打开 `http://127.0.0.1:8080`
2. 再打开第二个窗口或标签页访问同一个地址
3. 在任意一个窗口输入文本
4. 服务端会把前端提交的 text subtype 操作交给 `jsonot.Transform` 和 `jsonot.Apply`
5. 两个窗口都会收到最新文档内容

## 协议说明

前端向 `/ws` 发送的数据格式如下：

```json
{
  "type": "op",
  "version": 3,
  "op": [
    {"p": ["content"], "t": "text", "o": {"p": 4, "i": "abc"}}
  ]
}
```

服务端会返回：

- `init`：初始化文档和客户端标识
- `ack`：确认当前客户端提交的修改
- `update`：向其他客户端广播最新文档
- `error`：操作解析或应用失败

## 说明

这是一个尽量小的演示实现：

- 协同文档模型固定为 `{ "content": string }`
- 前端把一次连续编辑映射成 text subtype 插入 / 删除操作
- 为了简化状态管理，客户端在收到确认前会临时锁定输入

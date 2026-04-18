# jsonot/sharedb

`jsonot/sharedb` 提供一个轻量级的、ShareDB 风格的协同编辑后端抽象，基于 `jsonot` 的 OT 能力实现：

- 文档快照与版本号
- 客户端按版本提交操作（`Submit`）
- 服务端自动 rebase 并发操作（`Transform`）
- 订阅成功提交事件（`Subscribe`）

当前实现是内存版 `Store`，适合 demo、单机服务和二次封装。

## 安装

```bash
go get github.com/edocevol/jsonot/sharedb
```

## 快速开始

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/edocevol/jsonot/sharedb"
)

func main() {
	ctx := context.Background()
	store := sharedb.NewStore()

	_, _ = store.CreateDocument(ctx, "doc-1", json.RawMessage(`{"counter":0}`))

	result, _ := store.Submit(
		ctx,
		"doc-1",
		0,
		json.RawMessage(`[{"p":["counter"],"na":1}]`),
		"client-a",
	)

	fmt.Println(result.Version)        // 1
	fmt.Println(string(result.Document)) // {"counter":1}
}
```

## API

- `CreateDocument(ctx, documentID, initial)`：创建文档，初始版本为 `0`
- `GetSnapshot(ctx, documentID)`：获取最新快照
- `Submit(ctx, documentID, baseVersion, operation, source)`：提交并发操作
- `Subscribe(ctx, documentID, buffer)`：订阅提交成功事件

## 说明

- `Submit` 要求 `baseVersion` 在 `[0, currentVersion]` 范围内。
- 当 `baseVersion < currentVersion` 时，服务端会把操作与缺失区间的历史操作做 OT 转换。
- 订阅事件采用非阻塞投递，慢消费者可能丢事件；如需严格投递可在上层做持久队列。

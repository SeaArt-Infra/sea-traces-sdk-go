# Sealangfuse API Key 使用指南

本文档说明如何在 Go SDK 中使用 `sa-xxx` 形式的 Sealangfuse API Key 上报数据。用户不需要再传 public key 和 secret key，SDK 会在创建客户端时根据 `sa key` 和 `host` 自动解析出 Langfuse 项目凭证。

## 推荐配置

生产和测试环境都必须显式传入 `LANGFUSE_BASE_URL`。不要只传 `sa key`，因为同一个 `sa key` 可能在测试环境和生产环境中相同，SDK 需要根据 `host` 判断要向哪个 Sealangfuse 服务解析凭证并上报数据。

```bash
export SEALANGFUSE_API_KEY="sa-xxx"
export LANGFUSE_BASE_URL="https://sealangfuse-web.example.com"
```

如果部署环境的凭证查询接口不是默认路径，可以额外指定：

```bash
export SEALANGFUSE_CREDENTIALS_URL="https://sealangfuse-web.example.com/api/public/sea-project-api-credentials"
```

默认情况下 SDK 会请求：

```text
GET {LANGFUSE_BASE_URL}/api/public/sea-project-api-credentials?key={SEALANGFUSE_API_KEY}
```

## 从环境变量创建客户端

```go
package main

import (
	"context"
	"log"
	"time"

	langfuse "gitlab.internal.ops.haiyiai.tech/seaart-infra/sealangfus-sdk-go.git"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := langfuse.NewClientFromEnv(ctx)
	if err != nil {
		log.Fatalf("create langfuse client failed: %v", err)
	}
	defer client.Close()

	trace := client.StartTrace(ctx, "go-sdk-demo")
	span := trace.StartSpan("local-work")
	span.Input = map[string]any{"prompt": "ping"}
	span.Output = map[string]any{"answer": "pong"}
	span.End()
	trace.End()

	client.Flush()
}
```

`NewClientFromEnv` 会读取：

- `SEALANGFUSE_API_KEY`
- `LANGFUSE_BASE_URL`
- 可选 `SEALANGFUSE_CREDENTIALS_URL`

## 显式传参创建客户端

如果配置来自配置中心，可以直接传入：

```go
ctx := context.Background()

client, err := langfuse.NewClientWithAPIKey(
	ctx,
	"https://sealangfuse-web.example.com",
	"sa-xxx",
)
if err != nil {
	return err
}
defer client.Close()
```

如果 resolver 地址需要覆盖：

```go
client, err := langfuse.NewClientWithAPIKey(
	ctx,
	"https://sealangfuse-web.example.com",
	"sa-xxx",
	langfuse.WithCredentialsURL(
		"https://sealangfuse-web.example.com/api/public/sea-project-api-credentials",
	),
)
```

## 创建 trace、span 和 score

```go
package main

import (
	"context"
	"log"

	langfuse "gitlab.internal.ops.haiyiai.tech/seaart-infra/sealangfus-sdk-go.git"
	"gitlab.internal.ops.haiyiai.tech/seaart-infra/sealangfus-sdk-go.git/pkg/scores"
)

func main() {
	ctx := context.Background()

	client, err := langfuse.NewClientFromEnv(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	trace := client.StartTrace(ctx, "checkout-flow")
	trace.UserID = "user-123"
	trace.SessionID = "session-123"
	trace.Input = map[string]any{"step": "start"}

	span := trace.StartSpan("llm-call")
	span.Input = map[string]any{"prompt": "ping"}
	span.Output = map[string]any{"answer": "pong"}
	span.End()

	trace.Output = map[string]any{"ok": true}
	trace.End()

	_, err = client.Scores().Create(ctx, &scores.CreateScoreRequest{
		TraceID:  trace.ID,
		Name:     "quality",
		Value:    1.0,
		DataType: scores.ScoreDataTypeNumeric,
		Comment:  "created by Go SDK",
	})
	if err != nil {
		log.Fatal(err)
	}

	client.Flush()
}
```

## 与旧构造函数兼容

旧构造函数仍然保留：

```go
client := langfuse.NewClient(
	"https://sealangfuse-web.example.com",
	"pk-lf-xxx",
	"sk-lf-xxx",
)
```

如果继续使用 `NewClient(host, publicKey, secretKey)`，SDK 不会调用 Sealangfuse 凭证查询接口。

## 配置优先级

Go SDK 中推荐使用以下方式：

1. 旧方式：`NewClient(host, publicKey, secretKey)`
2. 新方式：`NewClientWithAPIKey(ctx, host, apiKey)`
3. 环境变量方式：`NewClientFromEnv(ctx)`

`NewClientWithAPIKey` 和 `NewClientFromEnv` 会返回 `(*Langfuse, error)`。resolver 失败时不会回退到默认云端地址，也不会创建一个使用空凭证的客户端。

## SDK 内部流程

创建客户端时 SDK 会执行以下步骤：

1. 读取传入的 `apiKey`，或从 `SEALANGFUSE_API_KEY` 读取
2. 读取传入的 `host`，或从 `LANGFUSE_BASE_URL` 读取
3. 拼出默认 resolver 地址，或使用 `WithCredentialsURL` / `SEALANGFUSE_CREDENTIALS_URL`
4. 请求 resolver 获取 `publicKey`、`secretKey`、`baseUrl`、`status`
5. 校验 `status == "ACTIVE"`，并要求 `publicKey`、`secretKey`、`baseUrl` 非空
6. 使用解析出的 `publicKey` 和 `secretKey` 配置原有 resty client
7. 最终上报地址仍使用用户传入的 `host`

resolver 返回的 `baseUrl` 只用于校验，不会覆盖用户传入的 `host`。

## 缓存和并发

SDK 不会每次上报都查询 resolver。凭证只会在创建客户端时解析，并且有进程内缓存。

缓存 key 为：

```text
(sa_key, credentials_url)
```

同一进程内多个 goroutine 并发使用相同 `sa key` 和 resolver 地址时，SDK 会合并并发请求，只有第一个调用实际访问 resolver，其他调用等待同一个结果。

## 自定义 HTTP Client

可以继续使用 `WithHTTPClient` 控制超时、代理、连接池等行为。该 HTTP client 会用于 resolver 请求，也会用于后续 Langfuse API 请求。

```go
httpClient := &http.Client{
	Timeout: 30 * time.Second,
}

client, err := langfuse.NewClientWithAPIKey(
	ctx,
	"https://sealangfuse-web.example.com",
	"sa-xxx",
	langfuse.WithHTTPClient(httpClient),
)
```

## 错误处理

常见错误和处理方式：

| 错误                                       | 原因                                  | 处理                                                  |
| ------------------------------------------ | ------------------------------------- | ----------------------------------------------------- |
| `SEALANGFUSE_API_KEY is required`          | 未配置 sa key                         | 设置 `SEALANGFUSE_API_KEY` 或传入 `apiKey`            |
| `LANGFUSE_BASE_URL is required`            | 使用环境变量创建客户端时没有指定环境  | 设置 `LANGFUSE_BASE_URL`                              |
| resolver 返回非 2xx                        | 凭证查询接口不可达或服务异常          | 检查 `LANGFUSE_BASE_URL`、网络和 Sealangfuse 服务状态 |
| `invalid Sealangfuse credentials response` | 返回缺字段或 `status` 不是 `ACTIVE`   | 检查 sa key 映射和项目凭证状态                        |
| 查询不到 trace                             | 数据未 flush 或 base URL 指向错误环境 | 调用 `Flush()`/`Close()`，确认 `LANGFUSE_BASE_URL`    |

错误信息会对 `sa key` 做脱敏处理，不会输出完整 `sa key` 或 `secretKey`。

## 从旧配置迁移

旧方式：

```go
client := langfuse.NewClient(
	"https://sealangfuse-web.example.com",
	"pk-lf-xxx",
	"sk-lf-xxx",
)
```

新方式：

```go
client, err := langfuse.NewClientWithAPIKey(
	ctx,
	"https://sealangfuse-web.example.com",
	"sa-xxx",
)
```

如果服务统一通过环境变量注入配置，可以使用：

```go
client, err := langfuse.NewClientFromEnv(ctx)
```

## 安全建议

- 不要把 `SEALANGFUSE_API_KEY` 提交到 Git。
- 不要在日志里打印完整 `sa key`、`publicKey`、`secretKey`。
- 测试环境和生产环境都显式配置 `LANGFUSE_BASE_URL`。
- 在服务启动阶段创建并复用 client，避免每个请求都创建新 client。

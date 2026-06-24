# Sea Traces Go SDK

Sea Traces 的 Go 客户端和 SDK。SDK 对外使用 Sea Traces 模块命名，支持内部项目直传和外部网关凭证两种初始化模式。

## 安装

目前依赖只能通过 GitHub module path 安装：

```bash
go get github.com/SeaArt-Infra/sea-traces-sdk-go
```

## 配置

Sea Traces Go SDK 支持两种推荐配置。内部用户直接配置项目和上报地址：

```bash
export SEATRACES_PROJECT_ID="project-id"
export SEATRACES_BASE_URL="https://your-upload.example.com"
```

外部用户使用网关凭证初始化，SDK 会先解析一次项目信息，再走无鉴权上报接口：

```bash
export SEA_TRACES_API_KEY="sea-traces-api-key"
export SEA_TRACES_BASE_URL="https://your-gateway.example.com"
```

默认的凭证解析请求为：

```text
POST {SEA_TRACES_BASE_URL}/hub/sea-traces-api-key
```

请求体只包含 `api_key` 和 `base_url`。网关返回 `project_id` 和 `base_url` 后，trace 上报会发送到 `{base_url}/api/public/ingestion-noauth`，请求体包含 `project_id` 和 `batch`。

旧的 `SEATRACES_PUBLIC_KEY`、`SEATRACES_SECRET_KEY`、`SEATRACES_BASE_URL` direct-basic-auth 配置仍保留兼容，但不再是推荐初始化方式。

## 使用方式

```go
package main

import (
    "context"
    "log"

    seatraces "github.com/SeaArt-Infra/sea-traces-sdk-go"
)

func main() {
    ctx := context.Background()

    client, err := seatraces.NewClientFromEnv(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    trace := client.StartTrace(ctx, "checkout-flow")
    span := trace.StartSpan("local-work")
    span.End()
    trace.End()

    client.Flush()
}
```

也可以显式传入配置：

```go
client, err := seatraces.NewClientWithConfig(
    ctx,
    seatraces.Config{
        APIKey:  "sea-traces-api-key",
        BaseURL: "https://your-gateway.example.com",
    },
)
```

如果需要覆盖凭证解析地址：

```go
client, err := seatraces.NewClientWithConfig(
    ctx,
    seatraces.Config{
        APIKey:  "sea-traces-api-key",
        BaseURL: "https://your-gateway.example.com",
    },
    seatraces.WithCredentialsURL(
        "https://your-gateway.example.com/hub/sea-traces-api-key",
    ),
)
```

`NewClientFromEnv` 会读取：

- `SEATRACES_PUBLIC_KEY`
- `SEATRACES_SECRET_KEY`
- `SEATRACES_PROJECT_ID`
- `SEATRACES_BASE_URL`
- `SEA_TRACES_API_KEY`
- `SEA_TRACES_BASE_URL`

## 上报模型

SDK 有两种上报模式：每次创建独立 trace，或在同一个 trace 里追加子 observation。

### 独立 trace

每次独立调用 `client.StartTrace(ctx, name)`，都会创建新的 trace：

```go
traceA := client.StartTrace(ctx, "task-a")
spanA := traceA.StartSpan("task-a")
spanA.Input = map[string]any{"step": "a"}
spanA.Output = map[string]any{"ok": true}
spanA.End()
traceA.End()

traceB := client.StartTrace(ctx, "task-b")
spanB := traceB.StartSpan("task-b")
spanB.Input = map[string]any{"step": "b"}
spanB.Output = map[string]any{"ok": true}
spanB.End()
traceB.End()

client.Flush()
```

上面示例会产生两个独立 trace：`task-a` 和 `task-b`。

### 在同一个 trace 里追加

如果要把一次业务流程里的多个步骤挂到同一个 trace 下，先创建一个 trace，再通过这个 trace 创建 span、generation 或其他 observation：

下面示例需要额外导入 `github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/traces` 和 `github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/scores`。

```go
trace := client.StartTrace(ctx, "checkout-flow")
trace.Input = map[string]any{"step": "start"}

span := trace.StartSpan("checkout-flow")
span.Input = map[string]any{"step": "start"}

generation := trace.StartGeneration("llm-call")
generation.Model = "demo-model"
generation.Input = []map[string]string{
    {"role": "user", "content": "ping"},
}
generation.Output = []map[string]string{
    {"role": "assistant", "content": "pong"},
}
generation.Usage = traces.Usage{
    Input:  1,
    Output: 1,
    Total:  2,
    Unit:   traces.UnitTokens,
}
generation.End()

span.Output = map[string]any{"ok": true}
span.End()

trace.Output = map[string]any{"ok": true}
trace.End()

_, err := client.Scores().Create(ctx, &scores.CreateScoreRequest{
    TraceID:  trace.ID,
    Name:     "quality",
    Value:    1.0,
    DataType: scores.ScoreDataTypeNumeric,
})
if err != nil {
    log.Fatal(err)
}

client.Flush()
```

上面示例里，`checkout-flow` 是一个 trace，`checkout-flow` span 和 `llm-call` generation 都归属到这个 trace 下。

调用 `trace.End()` 后，SDK 会结束当前 trace，并将该 trace 及其 observations 放入待上报队列。进程退出前需要调用 `client.Flush()` 或 `client.Close()`，确保待上报批次被发送出去。

## 安全

不要提交 `.env` 文件。不要在日志中打印完整 `SEA_TRACES_API_KEY`、`SEATRACES_SECRET_KEY`、解析后的 `secretKey` 或原始凭证响应。

## 开发

```bash
go build ./...
go test ./...
```

## 许可证

[MIT](LICENSE)

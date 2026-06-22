# Sea Traces Go SDK

Go client and SDK for Sea Traces. The SDK keeps the existing Langfuse ingestion and API pipeline internally while exposing Sea Traces module naming and required team-key authentication.

## Installation

```bash
go get github.com/SeaArt-Infra/sea-traces-sdk-go
```

## Configuration

The SDK requires both values at initialization time:

```bash
SEA_TEAM_KEY=sea-team-key
SEA_TRACES_BASE_URL=https://your-sea-traces.example.com
```

`SEALANGFUSE_API_KEY`, `LANGFUSE_BASE_URL`, and default Langfuse Cloud values do not enable the renamed SDK by themselves.

## Usage

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

Equivalent explicit configuration:

```go
client, err := seatraces.NewClientWithAPIKey(
    ctx,
    "https://your-sea-traces.example.com",
    "sea-team-key",
)
```

See [SEALANGFUSE_AUTH.md](./SEALANGFUSE_AUTH.md) for resolver details, cache behavior, and migration notes.

## Development

```bash
go build ./...
go test ./...
```

## License

[MIT](LICENSE)

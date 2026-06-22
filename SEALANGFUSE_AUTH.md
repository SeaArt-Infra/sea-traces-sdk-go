# Sea Traces Go SDK Auth Guide

The Sea Traces Go SDK uses a team key plus a Sea Traces base URL. Users do not pass Langfuse public and secret keys directly for normal SDK initialization. The SDK resolves project credentials once, then reuses the existing Langfuse ingestion/API pipeline internally.

## Required configuration

```bash
export SEA_TEAM_KEY="sea-team-key"
export SEA_TRACES_BASE_URL="https://sea-traces.example.com"
```

Optional resolver override:

```bash
export SEALANGFUSE_CREDENTIALS_URL="https://sea-traces.example.com/api/public/sea-project-api-credentials"
```

The default resolver request is:

```text
GET {SEA_TRACES_BASE_URL}/api/public/sea-project-api-credentials?key={SEA_TEAM_KEY}
```

`SEALANGFUSE_API_KEY`, `LANGFUSE_BASE_URL`, and default Langfuse Cloud values do not enable the renamed SDK by themselves.

## Environment-based initialization

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

    trace := client.StartTrace(ctx, "go-sdk-demo")
    span := trace.StartSpan("local-work")
    span.End()
    trace.End()

    client.Flush()
}
```

`NewClientFromEnv` reads:

- `SEA_TEAM_KEY`
- `SEA_TRACES_BASE_URL`
- optional `SEALANGFUSE_CREDENTIALS_URL`

## Explicit initialization

```go
client, err := seatraces.NewClientWithAPIKey(
    ctx,
    "https://sea-traces.example.com",
    "sea-team-key",
)
```

If the resolver endpoint must be overridden:

```go
client, err := seatraces.NewClientWithAPIKey(
    ctx,
    "https://sea-traces.example.com",
    "sea-team-key",
    seatraces.WithCredentialsURL(
        "https://sea-traces.example.com/api/public/sea-project-api-credentials",
    ),
)
```

## Internal flow

1. Read `apiKey` or `SEA_TEAM_KEY`.
2. Read `host` or `SEA_TRACES_BASE_URL`.
3. Return an error if either value is missing or blank.
4. Resolve credentials from `{host}/api/public/sea-project-api-credentials`, unless `WithCredentialsURL` or `SEALANGFUSE_CREDENTIALS_URL` overrides it.
5. Require `status == "ACTIVE"` and non-empty `publicKey`, `secretKey`, and `baseUrl`.
6. Configure the existing resty clients with the resolved public/secret keys.
7. Keep the user-provided `host` as the final API and ingestion base URL.

The resolver response `baseUrl` is validated but does not override the user-provided Sea Traces base URL.

## Cache and concurrency

Credential resolution is cached in-process by `(team_key, credentials_url)`. Concurrent callers for the same cache key share one in-flight resolver request.

## Safety

Do not commit `SEA_TEAM_KEY` or `.env` files. Do not log full team keys, resolved `secretKey`, or raw credential responses.

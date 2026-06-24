# Sea Traces Go SDK Auth Guide

The Sea Traces Go SDK supports project-based noauth ingestion without exposing Langfuse naming to users.

## Internal Project Mode

Internal users configure the target project and upload base URL directly:

```bash
export SEATRACES_PROJECT_ID="project-id"
export SEATRACES_BASE_URL="https://sea-traces-upload.example.com"
```

Equivalent constructor config:

```go
client, err := seatraces.NewClientWithConfig(ctx, seatraces.Config{
    ProjectID: "project-id",
    BaseURL:   "https://sea-traces-upload.example.com",
})
```

## Gateway Auth

External users provide gateway credentials. The SDK resolves project upload settings once and then uses noauth ingestion.

```bash
export SEA_TRACES_API_KEY="sea-traces-api-key"
export SEA_TRACES_BASE_URL="https://sea-traces-gateway.example.com"
```

Equivalent constructor config:

```go
client, err := seatraces.NewClientWithConfig(ctx, seatraces.Config{
    APIKey:  "sea-traces-api-key",
    BaseURL: "https://sea-traces-gateway.example.com",
})
```

The default resolver request is:

```text
POST {SEA_TRACES_BASE_URL}/hub/sea-traces-api-key
```

Request body:

```json
{
  "api_key": "sea-traces-api-key",
  "base_url": "https://sea-traces-gateway.example.com"
}
```

Expected response:

```json
{
  "project_id": "project-id",
  "base_url": "https://sea-traces-upload.example.com"
}
```

Trace ingestion is sent to:

```text
POST {base_url}/api/public/ingestion-noauth
```

with a body containing `project_id` and `batch`.

## Precedence

`NewClientFromEnv` uses this order:

1. Complete legacy direct env config: `SEATRACES_PUBLIC_KEY`, `SEATRACES_SECRET_KEY`, `SEATRACES_BASE_URL`.
2. Complete internal project env config: `SEATRACES_PROJECT_ID`, `SEATRACES_BASE_URL`.
3. Complete gateway env config: `SEA_TRACES_API_KEY`, `SEA_TRACES_BASE_URL`.
4. Complete legacy direct env fallback, if present.

`NewClientWithConfig` gives complete legacy direct constructor config precedence, then internal project config, then gateway config.

Incomplete direct credentials do not partially override gateway-resolved project settings.

## Cache and Concurrency

Credential resolution is cached in-process by `(api_key, credentials_url)`. Concurrent callers for the same cache key share one in-flight resolver request.

## Safety

Do not commit `.env` files. Do not log full gateway API keys or raw credential responses.

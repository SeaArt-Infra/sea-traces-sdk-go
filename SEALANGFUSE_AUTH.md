# Sea Traces Go SDK Auth Guide

The Sea Traces Go SDK supports two authentication modes without exposing Langfuse naming to users.

## Direct Upload

Internal users who already have upload credentials can configure the SDK directly:

```bash
export SEATRACES_PUBLIC_KEY="pk-..."
export SEATRACES_SECRET_KEY="sk-..."
export SEATRACES_BASE_URL="https://sea-traces-upload.example.com"
```

Equivalent constructor config:

```go
client, err := seatraces.NewClientWithConfig(ctx, seatraces.Config{
    PublicKey: "pk-...",
    SecretKey: "sk-...",
    BaseURL:   "https://sea-traces-upload.example.com",
})
```

## Gateway Auth

External users provide gateway credentials. The SDK resolves upload credentials once and then uses the same upload pipeline as direct mode.

```bash
export SEA_TRACES_API_KEY="sea-traces-api-key"
export SEA_TRACES_BASE_URL="https://sea-traces-gateway.example.com"
export SEA_TRACES_PROJECT_ID="project-id"
```

Equivalent constructor config:

```go
client, err := seatraces.NewClientWithConfig(ctx, seatraces.Config{
    APIKey:    "sea-traces-api-key",
    BaseURL:   "https://sea-traces-gateway.example.com",
    ProjectID: "project-id",
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
  "base_url": "https://sea-traces-gateway.example.com",
  "project_id": "project-id"
}
```

Expected response:

```json
{
  "publicKey": "pk-...",
  "secretKey": "sk-...",
  "baseUrl": "https://sea-traces-upload.example.com"
}
```

## Precedence

`NewClientFromEnv` uses this order:

1. Complete direct env config: `SEATRACES_PUBLIC_KEY`, `SEATRACES_SECRET_KEY`, `SEATRACES_BASE_URL`.
2. Complete gateway env config: `SEA_TRACES_API_KEY`, `SEA_TRACES_BASE_URL`, `SEA_TRACES_PROJECT_ID`.
3. Complete legacy direct env fallback, if present.

`NewClientWithConfig` gives complete direct constructor config precedence over gateway constructor config.

Incomplete direct credentials do not partially override gateway-resolved credentials. In gateway mode, the resolver response becomes the final upload credentials and base URL.

## Cache and Concurrency

Credential resolution is cached in-process by `(api_key, project_id, credentials_url)`. Concurrent callers for the same cache key share one in-flight resolver request.

## Safety

Do not commit `.env` files. Do not log full gateway API keys, upload secret keys, resolved credentials, or raw credential responses.

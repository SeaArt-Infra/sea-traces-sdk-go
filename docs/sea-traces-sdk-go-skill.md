---
name: sea-traces-sdk-go
description: Use the Sea Traces Go SDK for gateway-authenticated tracing, project management, trace queries, and batch ingestion.
type: slash_command
tags:
  - go
  - sea-traces
  - sdk
  - tracing
---

# Sea Traces Go SDK Complete Reference

Use this skill when a user needs to instrument a Go service, upload traces,
manage projects, query traces, inspect results, or diagnose gateway
authentication.

**Trigger scenarios:** Go tracing setup, project management, trace lookup,
batch ingestion, response inspection, or SDK troubleshooting.

**Processing rules:**

1. Prefer `NewAPIClient` for project and trace APIs.
2. Prefer the buffered tracing client for application instrumentation.
3. Include complete error handling and never print bearer tokens.
4. Use the response inspection examples when a user asks to view, copy, or
   save trace data.

**Output format:** Provide a runnable Go snippet with a short explanation. Use
`seatraces "github.com/SeaArt-Infra/sea-traces-sdk-go"` as the import alias.

## Install

```bash
go get github.com/SeaArt-Infra/sea-traces-sdk-go
```

## Gateway API client

```go
api, err := seatraces.NewAPIClient("https://gateway.example.com", "token")
if err != nil { return err }

projects, err := api.ListProjects(ctx)
project, err := api.CreateProject(ctx, seatraces.CreateProjectRequest{Name: "checkout"})
_, err = api.UpdateProject(ctx, project.ID, seatraces.UpdateProjectRequest{Name: "checkout-v2"})
traces, err := api.ListTraces(ctx, project.ID, seatraces.TraceQuery{TraceID: "trace-id"})
_, err = api.Ingest(ctx, project.ID, []traces.IngestionEvent{event})
```

The client sends `Authorization: Bearer <token>` and uses the internal project
API routes. A missing project ID, invalid token, or non-2xx response is an
error and should be returned to the caller.

## Inspect, save, and copy results

```go
result, err := api.ListTraces(ctx, projectID, seatraces.TraceQuery{Limit: 10})
if err != nil { return err }

for _, item := range result.Data {
    fmt.Println(item.ID, item.Name, item.Timestamp)
}

payload, err := json.MarshalIndent(result, "", "  ")
if err != nil { return err }
if err := os.WriteFile("traces.json", payload, 0600); err != nil { return err }
fmt.Println("Copy this trace ID:", result.Data[0].ID)
```

Use `os.ReadFile` and `json.Unmarshal` to load a saved response. Do not save
tokens or sensitive trace payloads in public locations.

## Tracing client

Use `NewClientWithConfig` for the existing buffered tracing client. Call
`Flush` before process shutdown and `Close` when the client owns the worker.

```go
client, err := seatraces.NewClientWithConfig(ctx, seatraces.Config{
    ProjectID: "project-id",
    BaseURL:   "https://gateway.example.com",
})
```

Prefer the gateway configuration for new integrations and never log tokens or
raw credential responses.

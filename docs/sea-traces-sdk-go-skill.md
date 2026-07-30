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

# Sea Traces Go SDK Skill

Use this skill when a user needs to instrument a Go service, upload traces,
manage projects, query traces, or diagnose gateway authentication.

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

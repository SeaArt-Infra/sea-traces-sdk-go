# Sea Traces Go SDK

The Sea Traces Go SDK provides buffered tracing plus a gateway-authenticated
client for projects, trace queries, and batch ingestion.

## Install

```bash
go get github.com/SeaArt-Infra/sea-traces-sdk-go
```

## Gateway API

```go
api, err := seatraces.NewAPIClient("https://gateway.example.com", "token")
if err != nil { log.Fatal(err) }

projects, err := api.ListProjects(ctx)
project, err := api.CreateProject(ctx, seatraces.CreateProjectRequest{Name: "checkout"})
_, err = api.UpdateProject(ctx, project.ID, seatraces.UpdateProjectRequest{Name: "checkout-v2"})
result, err := api.ListTraces(ctx, project.ID, seatraces.TraceQuery{TraceID: "trace-id"})
_, err = api.Ingest(ctx, project.ID, events)
```

`NewAPIClient` requires a `baseURL` and `token`. Requests use
`Authorization: Bearer <token>` and these routes:

```text
GET    /api/internal/projects
POST   /api/internal/projects
PATCH  /api/internal/projects/{projectId}
POST   /api/internal/projects/{projectId}/ingestion
GET    /api/internal/projects/{projectId}/traces
```

Trace queries support `TraceID`, `FromTimestamp`, `ToTimestamp`, `Page`, and
`Limit`. Without timestamps, the server returns the most recent 24 hours.

## Buffered tracing

```go
client, err := seatraces.NewClientWithConfig(ctx, seatraces.Config{
    ProjectID: "project-id",
    BaseURL:   "https://gateway.example.com",
})
if err != nil { log.Fatal(err) }
defer client.Close()

trace := client.StartTrace(ctx, "checkout")
trace.Input = map[string]any{"cart": "cart-1"}
trace.Output = map[string]any{"status": "ok"}
trace.End()
client.Flush()
```

## Development

```bash
go build ./...
go test ./...
```

Do not commit or log bearer tokens or raw credential responses.

See [`docs/sea-traces-sdk-go-skill.md`](docs/sea-traces-sdk-go-skill.md) for
agent-assisted usage guidance.

## License

[MIT](LICENSE)

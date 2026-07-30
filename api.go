package langfuse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/traces"
	"github.com/go-resty/resty/v2"
)

// APIClient accesses the Sea Traces project and trace APIs through a gateway.
// The gateway authenticates the bearer token before forwarding the request.
type APIClient struct{ client *resty.Client }

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type CreateProjectRequest struct {
	Name string `json:"name"`
}
type UpdateProjectRequest struct {
	Name string `json:"name"`
}
type ProjectsResponse struct {
	Data []Project `json:"data"`
}
type TraceQuery struct {
	TraceID       string
	FromTimestamp *time.Time
	ToTimestamp   *time.Time
	Page          int
	Limit         int
}
type Trace struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Timestamp time.Time `json:"timestamp"`
	Name      string    `json:"name"`
	Input     any       `json:"input"`
	Output    any       `json:"output"`
	UserID    string    `json:"userId"`
	SessionID string    `json:"sessionId"`
}
type TracesResponse struct {
	Data []Trace `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		Limit      int `json:"limit"`
		TotalItems int `json:"totalItems"`
		TotalPages int `json:"totalPages"`
	} `json:"meta"`
}
type IngestionResponse struct {
	Successes []map[string]any `json:"successes"`
	Errors    []map[string]any `json:"errors"`
}

// NewAPIClient creates a gateway-authenticated API client.
func NewAPIClient(baseURL, token string) (*APIClient, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(token) == "" {
		return nil, errors.New("baseURL and token are required")
	}
	return &APIClient{client: resty.New().SetBaseURL(strings.TrimRight(baseURL, "/")).SetHeader("Authorization", "Bearer "+token)}, nil
}

func (c *APIClient) request(ctx context.Context) *resty.Request { return c.client.R().SetContext(ctx) }
func apiError(r *resty.Response, err error) error {
	if err != nil {
		return err
	}
	if r.IsError() {
		return fmt.Errorf("sea traces API request failed: %s", r.String())
	}
	return nil
}

func (c *APIClient) ListProjects(ctx context.Context) (*ProjectsResponse, error) {
	var out ProjectsResponse
	r, e := c.request(ctx).SetResult(&out).Get("/api/internal/projects")
	return &out, apiError(r, e)
}
func (c *APIClient) CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error) {
	var out Project
	r, e := c.request(ctx).SetBody(req).SetResult(&out).Post("/api/internal/projects")
	return &out, apiError(r, e)
}
func (c *APIClient) UpdateProject(ctx context.Context, id string, req UpdateProjectRequest) (*Project, error) {
	var out Project
	r, e := c.request(ctx).SetBody(req).SetResult(&out).SetPathParam("projectId", id).Patch("/api/internal/projects/{projectId}")
	return &out, apiError(r, e)
}
func (c *APIClient) Ingest(ctx context.Context, projectID string, batch []traces.IngestionEvent) (*IngestionResponse, error) {
	var out IngestionResponse
	r, e := c.request(ctx).SetBody(map[string]any{"batch": batch}).SetResult(&out).SetPathParam("projectId", projectID).Post("/api/internal/projects/{projectId}/ingestion")
	return &out, apiError(r, e)
}
func (c *APIClient) ListTraces(ctx context.Context, projectID string, q TraceQuery) (*TracesResponse, error) {
	var out TracesResponse
	page := q.Page
	if page < 1 {
		page = 1
	}
	limit := q.Limit
	if limit < 1 {
		limit = 50
	}
	req := c.request(ctx).SetResult(&out).SetPathParam("projectId", projectID).SetQueryParam("page", fmt.Sprint(page)).SetQueryParam("limit", fmt.Sprint(limit))
	if q.TraceID != "" {
		req.SetQueryParam("traceId", q.TraceID)
	}
	if q.FromTimestamp != nil {
		req.SetQueryParam("fromTimestamp", q.FromTimestamp.Format(time.RFC3339Nano))
	}
	if q.ToTimestamp != nil {
		req.SetQueryParam("toTimestamp", q.ToTimestamp.Format(time.RFC3339Nano))
	}
	r, e := req.Get("/api/internal/projects/{projectId}/traces")
	return &out, apiError(r, e)
}

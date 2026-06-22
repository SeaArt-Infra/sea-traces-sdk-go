// Package langfuse provides a Go client library for interacting with Sea Traces.
//
// This package offers comprehensive support for observability tracing, prompt management,
// model configuration, datasets, sessions, scores, projects, LLM connections, comments,
// and annotations functionality with efficient batch processing.
//
// Basic usage:
//
//	client, err := langfuse.NewClientFromEnv(context.Background())
//	if err != nil {
//		return err
//	}
//	defer client.Close()
//
//	trace := client.StartTrace("my-application")
//	span := trace.StartSpan("processing-step")
//	// ... your application logic
//	span.End()
//	trace.End()
package langfuse

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"

	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/organizations"

	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/annotations"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/comments"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/datasets"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/health"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/llmconnections"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/media"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/metrics"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/models"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/projects"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/prompts"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/scores"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/sessions"
	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/traces"
)

// Langfuse is the main client for interacting with the Langfuse platform.
//
// It provides access to all Langfuse functionality including tracing, prompts,
// models, datasets, sessions, scores, projects, LLM connections, comments,
// and annotations through dedicated client instances.
//
// The client manages HTTP connections and provides efficient batch processing
// for trace ingestion with automatic flushing and graceful shutdown capabilities.
type Langfuse struct {
	ingestor      *traces.Ingestor
	prompt        *prompts.Client
	model         *models.Client
	project       *projects.Client
	comment       *comments.Client
	dataset       *datasets.Client
	session       *sessions.Client
	score         *scores.Client
	llmConnection *llmconnections.Client
	organization  *organizations.Client
	health        *health.Client
	media         *media.Client
	metric        *metrics.Client
	annotationQ   *annotations.QueueClient
	annotationI   *annotations.ItemClient
	restyCli      *resty.Client
}

// ClientOption is a function that configures a Langfuse client.
type ClientOption func(*clientConfig)

// clientConfig holds configuration options for the Langfuse client.
type clientConfig struct {
	httpClient     *http.Client
	credentialsURL string
}

// WithHTTPClient sets a custom HTTP client for the Langfuse client.
//
// This allows you to customize timeout settings, transport configuration,
// and other HTTP client behavior. If not provided, resty will use its default HTTP client.
//
// Example:
//
//	httpClient := &http.Client{
//		Timeout: 30 * time.Second,
//		Transport: &http.Transport{
//			MaxIdleConns:        100,
//			MaxIdleConnsPerHost: 10,
//		},
//	}
//	client, err := langfuse.NewClientWithAPIKey(ctx, baseURL, teamKey, langfuse.WithHTTPClient(httpClient))
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(config *clientConfig) {
		config.httpClient = httpClient
	}
}

// WithCredentialsURL overrides the Sealangfuse credential resolver endpoint.
//
// By default, NewClientWithAPIKey resolves credentials from:
//
//	{host}/api/public/sea-project-api-credentials
func WithCredentialsURL(credentialsURL string) ClientOption {
	return func(config *clientConfig) {
		config.credentialsURL = credentialsURL
	}
}

// NewClient creates a new Langfuse client instance with resolved project credentials.
//
// Deprecated: use NewClientWithAPIKey or NewClientFromEnv. This compatibility
// constructor still requires SEA_TEAM_KEY and SEA_TRACES_BASE_URL to be set so
// the SDK cannot be used without the required Sea Traces configuration.
//
// The client automatically configures HTTP basic authentication and sets the API base URL.
// Remember to call Close() when done to ensure all pending traces are flushed.
//
// Example with custom HTTP client:
//
//	httpClient := &http.Client{Timeout: 30 * time.Second}
func NewClient(host string, publicKey string, secretKey string, options ...ClientOption) *Langfuse {
	mustHaveSeaTracesConfig()
	return newClientWithCredentials(host, publicKey, secretKey, options...)
}

// NewClientWithAPIKey creates a new Sea Traces client by resolving project credentials
// from a Sea Traces team key.
//
// The host is the final Sea Traces base URL used for ingestion and API requests.
// The resolver response baseUrl is validated but does not override host.
func NewClientWithAPIKey(ctx context.Context, host string, apiKey string, options ...ClientOption) (*Langfuse, error) {
	host = strings.TrimSpace(host)
	apiKey = strings.TrimSpace(apiKey)
	if host == "" {
		return nil, errMissingBaseURL
	}
	if apiKey == "" {
		return nil, errMissingAPIKey
	}

	config := &clientConfig{}
	for _, option := range options {
		option(config)
	}

	credentials, err := resolveSealangfuseCredentials(ctx, resolveSealangfuseCredentialsParams{
		apiKey:         apiKey,
		baseURL:        host,
		credentialsURL: config.credentialsURL,
		httpClient:     config.httpClient,
	})
	if err != nil {
		return nil, err
	}

	return newClientWithCredentials(host, credentials.PublicKey, credentials.SecretKey, options...), nil
}

// NewClientFromEnv creates a new Sea Traces client from SEA_TEAM_KEY and SEA_TRACES_BASE_URL.
func NewClientFromEnv(ctx context.Context, options ...ClientOption) (*Langfuse, error) {
	host := getSeaTracesBaseURLEnv()
	apiKey := getSeaTeamKeyEnv()
	if host == "" {
		return nil, errMissingBaseURL
	}
	if apiKey == "" {
		return nil, errMissingAPIKey
	}

	return NewClientWithAPIKey(ctx, host, apiKey, options...)
}

func mustHaveSeaTracesConfig() {
	if getSeaTracesBaseURLEnv() == "" {
		panic(errMissingBaseURL)
	}
	if getSeaTeamKeyEnv() == "" {
		panic(errMissingAPIKey)
	}
}

func newClientWithCredentials(host string, publicKey string, secretKey string, options ...ClientOption) *Langfuse {
	config := &clientConfig{}
	for _, option := range options {
		option(config)
	}

	var restyCli *resty.Client
	if config.httpClient != nil {
		restyCli = resty.NewWithClient(config.httpClient)
	} else {
		restyCli = resty.New()
	}

	restyCli.SetBaseURL(host+"/api/public").
		SetBasicAuth(publicKey, secretKey)

	return &Langfuse{
		ingestor:      traces.NewIngestor(restyCli),
		prompt:        prompts.NewClient(restyCli),
		model:         models.NewClient(restyCli),
		project:       projects.NewClient(restyCli),
		comment:       comments.NewClient(restyCli),
		dataset:       datasets.NewClient(restyCli),
		session:       sessions.NewClient(restyCli),
		score:         scores.NewClient(restyCli),
		llmConnection: llmconnections.NewClient(restyCli),
		organization:  organizations.NewClient(restyCli),
		health:        health.NewClient(restyCli),
		media:         media.NewClient(restyCli),
		metric:        metrics.NewClient(restyCli),
		annotationQ:   annotations.NewQueueClient(restyCli),
		annotationI:   annotations.NewItemClient(restyCli),
		restyCli:      restyCli,
	}
}

func (c *Langfuse) Flush() {
	c.ingestor.Flush()
}

// StartTrace creates a new trace with the given name.
//
// A trace represents a single execution flow in your application and can contain
// multiple observations (spans). Traces are automatically batched and sent to
// Langfuse for efficient ingestion.
//
// Returns a Trace instance that you can use to add observations and metadata.
func (c *Langfuse) StartTrace(ctx context.Context, name string) *traces.Trace {
	return c.ingestor.StartTrace(ctx, name)
}

// Prompts returns a client for managing prompt templates and versions.
//
// Use this client to create, retrieve, list, and manage prompt templates
// for your AI applications.
func (c *Langfuse) Prompts() *prompts.Client {
	return c.prompt
}

// Models returns a client for managing model configurations and pricing.
//
// Use this client to define model pricing, match patterns, and manage
// model metadata for cost tracking and analytics.
func (c *Langfuse) Models() *models.Client {
	return c.model
}

// Projects returns a client for managing projects and API keys.
//
// Use this client to create, update, and manage projects, as well as
// manage API keys within projects. Most operations require organization-scoped API keys.
func (c *Langfuse) Projects() *projects.Client {
	return c.project
}

// Comments returns a client for managing comments on traces, observations, and sessions.
//
// Use this client to add contextual comments to your traces and observations
// for collaboration and debugging purposes.
func (c *Langfuse) Comments() *comments.Client {
	return c.comment
}

// Datasets returns a client for managing datasets and dataset items.
//
// Use this client to create and manage datasets for training, evaluation,
// and testing of your AI models, including dataset items and runs.
func (c *Langfuse) Datasets() *datasets.Client {
	return c.dataset
}

// Sessions returns a client for managing user sessions and their associated traces.
//
// Use this client to retrieve and analyze user sessions, including
// filtering by time ranges and environments.
func (c *Langfuse) Sessions() *sessions.Client {
	return c.session
}

// Scores returns a client for managing evaluation scores and score configurations.
//
// Use this client to create, retrieve, and manage scores for your traces
// and observations, including score configurations for different data types.
func (c *Langfuse) Scores() *scores.Client {
	return c.score
}

// LLMConnections returns a client for managing LLM provider connections.
//
// Use this client to configure connections to various LLM providers
// like OpenAI, Anthropic, Azure OpenAI, AWS Bedrock, and Google Vertex AI.
func (c *Langfuse) LLMConnections() *llmconnections.Client {
	return c.llmConnection
}

// Organizations returns a client for managing organization and project memberships.
//
// Use this client to manage user roles and permissions within organizations
// and projects. Most operations require organization-scoped API keys.
func (c *Langfuse) Organizations() *organizations.Client {
	return c.organization
}

// Health returns a client for checking API health status and version.
//
// Use this client to verify connectivity and server status.
func (c *Langfuse) Health() *health.Client {
	return c.health
}

// Media returns a client for managing media files associated with traces and observations.
//
// Use this client to upload, retrieve, and manage media files including images, audio,
// video, and documents. Media files are associated with traces and observations through
// their input, output, or metadata fields.
func (c *Langfuse) Media() *media.Client {
	return c.media
}

// Metrics returns a client for querying Langfuse metrics data.
//
// Use this client to run aggregated analytics queries across traces,
// observations, and scores through the public metrics API.
func (c *Langfuse) Metrics() *metrics.Client {
	return c.metric
}

func (c *Langfuse) AnnotationQueues() *annotations.QueueClient {
	return c.annotationQ
}

func (c *Langfuse) AnnotationItems() *annotations.ItemClient {
	return c.annotationI
}

// Close gracefully shuts down the client and flushes all pending traces.
//
// This method ensures that all batched traces are sent to Langfuse before
// the client is closed. It should be called when you're done using the client,
// typically in a defer statement.
//
// Returns an error if the shutdown process fails or times out.
func (c *Langfuse) Close() error {
	return c.ingestor.Close()
}

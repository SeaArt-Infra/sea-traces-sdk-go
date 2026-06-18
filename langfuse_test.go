package langfuse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.internal.ops.haiyiai.tech/seaart-infra/sealangfus-sdk-go.git/pkg/traces"
)

func TestNewClient_WithoutOptions(t *testing.T) {
	client := NewClient("https://cloud.langfuse.com", "public-key", "secret-key")

	require.NotNil(t, client)
	require.NotNil(t, client.restyCli)
	require.NotNil(t, client.ingestor)
	require.NotNil(t, client.prompt)
	require.NotNil(t, client.model)
	require.NotNil(t, client.project)
	require.NotNil(t, client.comment)
	require.NotNil(t, client.dataset)
	require.NotNil(t, client.session)
	require.NotNil(t, client.score)
	require.NotNil(t, client.llmConnection)
	require.NotNil(t, client.organization)
	require.NotNil(t, client.health)
	require.NotNil(t, client.media)
	require.NotNil(t, client.metric)
	require.NotNil(t, client.Metrics())
}

func TestNewClient_WithHTTPClient(t *testing.T) {
	customHTTPClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
		},
	}

	client := NewClient("https://cloud.langfuse.com", "public-key", "secret-key", WithHTTPClient(customHTTPClient))

	require.NotNil(t, client)
	require.NotNil(t, client.restyCli)

	// Verify that the custom HTTP client is being used
	restyHTTPClient := client.restyCli.GetClient()
	require.Equal(t, customHTTPClient, restyHTTPClient)
	require.Equal(t, 30*time.Second, restyHTTPClient.Timeout)

	// Verify that all subclients are properly initialized
	require.NotNil(t, client.ingestor)
	require.NotNil(t, client.prompt)
	require.NotNil(t, client.model)
	require.NotNil(t, client.project)
	require.NotNil(t, client.comment)
	require.NotNil(t, client.dataset)
	require.NotNil(t, client.session)
	require.NotNil(t, client.score)
	require.NotNil(t, client.llmConnection)
	require.NotNil(t, client.organization)
	require.NotNil(t, client.health)
	require.NotNil(t, client.media)
	require.NotNil(t, client.metric)
	require.NotNil(t, client.Metrics())
}

func TestNewClient_WithMultipleOptions(t *testing.T) {
	customHTTPClient := &http.Client{
		Timeout: 45 * time.Second,
	}

	client := NewClient("https://cloud.langfuse.com", "public-key", "secret-key", WithHTTPClient(customHTTPClient))

	require.NotNil(t, client)

	// Verify that the custom HTTP client is being used
	restyHTTPClient := client.restyCli.GetClient()
	require.Equal(t, customHTTPClient, restyHTTPClient)
	require.Equal(t, 45*time.Second, restyHTTPClient.Timeout)
}

func TestWithHTTPClient(t *testing.T) {
	customHTTPClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	config := &clientConfig{}
	option := WithHTTPClient(customHTTPClient)
	option(config)

	require.Equal(t, customHTTPClient, config.httpClient)
}

func TestClientConfig_Default(t *testing.T) {
	config := &clientConfig{}
	require.Nil(t, config.httpClient)
}

func TestNewClientWithAPIKey(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	var resolverCalls int32
	var healthAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/public/sea-project-api-credentials":
			atomic.AddInt32(&resolverCalls, 1)
			require.Equal(t, "sa-test", r.URL.Query().Get("key"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"publicKey": "pk-lf-resolved",
				"secretKey": "sk-lf-resolved",
				"baseUrl":   "https://resolver-response.example.com",
				"status":    "ACTIVE",
			})
		case "/api/public/health":
			healthAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"version": "test",
				"status":  "OK",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClientWithAPIKey(context.Background(), server.URL, "sa-test")
	require.NoError(t, err)
	require.NotNil(t, client)

	health, err := client.Health().Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, "OK", health.Status)
	require.Equal(t, int32(1), atomic.LoadInt32(&resolverCalls))
	require.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-lf-resolved:sk-lf-resolved")), healthAuth)
}

func TestNewClientWithAPIKey_UsesCredentialsURLOverride(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	resolver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/credentials", r.URL.Path)
		require.Equal(t, "sa-test", r.URL.Query().Get("key"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publicKey": "pk-lf-override",
			"secretKey": "sk-lf-override",
			"baseUrl":   "https://resolver-response.example.com",
			"status":    "ACTIVE",
		})
	}))
	defer resolver.Close()

	client, err := NewClientWithAPIKey(
		context.Background(),
		"https://langfuse.example.com",
		"sa-test",
		WithCredentialsURL(resolver.URL+"/credentials"),
	)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "https://langfuse.example.com/api/public", client.restyCli.BaseURL)
}

func TestNewClientWithAPIKey_InactiveCredentials(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publicKey": "pk-lf-resolved",
			"secretKey": "sk-lf-resolved",
			"baseUrl":   "https://resolver-response.example.com",
			"status":    "INACTIVE",
		})
	}))
	defer server.Close()

	client, err := NewClientWithAPIKey(context.Background(), server.URL, "sa-test")
	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientWithAPIKey_DeduplicatesConcurrentResolution(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	var resolverCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&resolverCalls, 1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publicKey": "pk-lf-resolved",
			"secretKey": "sk-lf-resolved",
			"baseUrl":   "https://resolver-response.example.com",
			"status":    "ACTIVE",
		})
	}))
	defer server.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := NewClientWithAPIKey(context.Background(), server.URL, "sa-test")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&resolverCalls))
}

func TestNewClientFromEnv(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/public/sea-project-api-credentials", r.URL.Path)
		require.Equal(t, "sa-env", r.URL.Query().Get("key"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publicKey": "pk-lf-env",
			"secretKey": "sk-lf-env",
			"baseUrl":   "https://resolver-response.example.com",
			"status":    "ACTIVE",
		})
	}))
	defer server.Close()

	t.Setenv("SEALANGFUSE_API_KEY", "sa-env")
	t.Setenv("LANGFUSE_BASE_URL", server.URL)

	client, err := NewClientFromEnv(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestTrace(t *testing.T) {
	// Use test environment configuration instead of real environment sensitive information
	client := NewClient("http://localhost:3000", "test-public-key", "test-secret-key")

	// Create a trace
	trace := client.StartTrace(context.Background(), "Test Trace")
	trace.Input = map[string]string{"input": "Test input"}
	trace.Output = map[string]string{"output": "Test output"}
	trace.Tags = []string{"test", "example"}

	// Test Agent type observation
	agent := trace.StartObservation("test_agent", traces.ObservationTypeAgent)
	agent.Input = map[string]string{"input": "Test agent input"}
	agent.Output = map[string]string{"output": "Test agent output"}

	// Test Retriever type observation
	retriever := trace.StartObservation("test_retriever", traces.ObservationTypeRetriever)
	retriever.Input = map[string]string{"input": "Test retriever input"}
	retriever.Output = map[string]string{"output": "Test retriever output"}
	retriever.End()

	// Test Generation type observation (LLM)
	llm := trace.StartGeneration("test_generation")
	llm.Input = map[string]string{"input": "Test generation input"}
	llm.Output = map[string]string{"output": "Test generation output"}
	llm.Usage = traces.Usage{
		Input:  10,
		Output: 20,
		Total:  30,
		Unit:   traces.UnitTokens,
	}
	llm.End()

	// Test Tool type observation
	tool := trace.StartObservation("test_tool", traces.ObservationTypeTool)
	tool.Input = map[string]string{"input": "Test tool input"}
	tool.Output = map[string]string{"output": "Test tool output"}
	tool.End()

	// End Agent observation
	agent.End()

	// End trace
	trace.End()

	// Flush client to ensure all data is processed
	client.Flush()

	// Verify trace was created correctly
	require.NotEmpty(t, trace.ID, "Trace ID should not be empty")
	require.Equal(t, "Test Trace", trace.Name, "Trace name should match")

	// Verify trace fields
	require.Equal(t, map[string]string{"input": "Test input"}, trace.Input, "Trace input should match")
	require.Equal(t, map[string]string{"output": "Test output"}, trace.Output, "Trace output should match")
	require.Equal(t, []string{"test", "example"}, trace.Tags, "Trace tags should match")

	// Verify each observation type was created with correct parameters
	// Agent observation
	require.Equal(t, "test_agent", agent.Name, "Agent name should match")
	require.Equal(t, traces.ObservationTypeAgent, agent.Type, "Agent type should be AGENT")
	require.Equal(t, map[string]string{"input": "Test agent input"}, agent.Input, "Agent input should match")
	require.Equal(t, map[string]string{"output": "Test agent output"}, agent.Output, "Agent output should match")

	// Retriever observation
	require.Equal(t, "test_retriever", retriever.Name, "Retriever name should match")
	require.Equal(t, traces.ObservationTypeRetriever, retriever.Type, "Retriever type should be RETRIEVER")
	require.Equal(t, map[string]string{"input": "Test retriever input"}, retriever.Input, "Retriever input should match")
	require.Equal(t, map[string]string{"output": "Test retriever output"}, retriever.Output, "Retriever output should match")

	// Generation observation
	require.Equal(t, "test_generation", llm.Name, "Generation name should match")
	require.Equal(t, traces.ObservationTypeGeneration, llm.Type, "Generation type should be GENERATION")
	require.Equal(t, map[string]string{"input": "Test generation input"}, llm.Input, "Generation input should match")
	require.Equal(t, map[string]string{"output": "Test generation output"}, llm.Output, "Generation output should match")
	require.Equal(t, 10, llm.Usage.Input, "Generation input usage should match")
	require.Equal(t, 20, llm.Usage.Output, "Generation output usage should match")
	require.Equal(t, 30, llm.Usage.Total, "Generation total usage should match")
	require.Equal(t, traces.UnitTokens, llm.Usage.Unit, "Generation usage unit should match")

	// Tool observation
	require.Equal(t, "test_tool", tool.Name, "Tool name should match")
	require.Equal(t, traces.ObservationTypeTool, tool.Type, "Tool type should be TOOL")
	require.Equal(t, map[string]string{"input": "Test tool input"}, tool.Input, "Tool input should match")
	require.Equal(t, map[string]string{"output": "Test tool output"}, tool.Output, "Tool output should match")

	// Verify observations ended correctly (end time should be set)
	require.NotNil(t, retriever.EndTime, "Retriever end time should be set")
	require.NotNil(t, llm.EndTime, "Generation end time should be set")
	require.NotNil(t, tool.EndTime, "Tool end time should be set")
	require.NotNil(t, agent.EndTime, "Agent end time should be set")

	// Verify observation durations are reasonable (greater than or equal to 0)
	retrieverDuration := retriever.EndTime.Sub(retriever.StartTime)
	require.True(t, retrieverDuration >= 0, "Retriever duration should be non-negative")

	llmDuration := llm.EndTime.Sub(llm.StartTime)
	require.True(t, llmDuration >= 0, "Generation duration should be non-negative")

	toolDuration := tool.EndTime.Sub(tool.StartTime)
	require.True(t, toolDuration >= 0, "Tool duration should be non-negative")

	agentDuration := agent.EndTime.Sub(agent.StartTime)
	require.True(t, agentDuration >= 0, "Agent duration should be non-negative")
}

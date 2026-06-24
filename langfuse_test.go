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

	"github.com/SeaArt-Infra/sea-traces-sdk-go/pkg/traces"
)

func assertClientInitialized(t *testing.T, client *Langfuse) {
	t.Helper()

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
	require.NotNil(t, client.AnnotationQueues())
	require.NotNil(t, client.AnnotationItems())
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func TestNewClient_WithoutOptions(t *testing.T) {
	client := NewClient("https://cloud.langfuse.com", "public-key", "secret-key")

	assertClientInitialized(t, client)
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

	assertClientInitialized(t, client)
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

func TestNewClientWithConfig_DirectConfigSkipsResolver(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	var resolverCalls int32
	var healthAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hub/sea-traces-api-key":
			atomic.AddInt32(&resolverCalls, 1)
			http.Error(w, "resolver should not be called", http.StatusInternalServerError)
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

	client, err := NewClientWithConfig(context.Background(), Config{
		PublicKey: "pk-direct",
		SecretKey: "sk-direct",
		BaseURL:   server.URL,
		APIKey:    "gateway-ignored",
		ProjectID: "project-ignored",
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	health, err := client.Health().Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, "OK", health.Status)
	require.Equal(t, int32(0), atomic.LoadInt32(&resolverCalls))
	require.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-direct:sk-direct")), healthAuth)
}

func TestNewClientFromEnv_DirectEnvSkipsResolver(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	var resolverCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hub/sea-traces-api-key":
			atomic.AddInt32(&resolverCalls, 1)
			http.Error(w, "resolver should not be called", http.StatusInternalServerError)
		case "/api/public/health":
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

	t.Setenv("SEATRACES_PUBLIC_KEY", "pk-env")
	t.Setenv("SEATRACES_SECRET_KEY", "sk-env")
	t.Setenv("SEATRACES_BASE_URL", server.URL)
	t.Setenv("SEA_TRACES_API_KEY", "gateway-ignored")
	t.Setenv("SEA_TRACES_BASE_URL", server.URL)
	t.Setenv("SEA_TRACES_PROJECT_ID", "project-ignored")

	client, err := NewClientFromEnv(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, int32(0), atomic.LoadInt32(&resolverCalls))
	require.Equal(t, server.URL+"/api/public", client.restyCli.BaseURL)
}

func TestNewClientWithConfig_GatewayConfigPostsResolverBody(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	var resolverCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hub/sea-traces-api-key":
			atomic.AddInt32(&resolverCalls, 1)
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, map[string]string{
				"api_key":  "sa-test",
				"base_url": serverURL(r),
			}, body)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"project_id": "project-123",
				"base_url":   serverURL(r),
			})
		case "/api/public/health":
			require.Empty(t, r.Header.Get("Authorization"))
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

	client, err := NewClientWithConfig(context.Background(), Config{
		APIKey:  "sa-test",
		BaseURL: server.URL,
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	health, err := client.Health().Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, "OK", health.Status)
	require.Equal(t, int32(1), atomic.LoadInt32(&resolverCalls))
	require.Equal(t, server.URL+"/api/public", client.restyCli.BaseURL)
}

func TestNewClientWithConfig_APIKeyTakesGatewayPathWhenProjectIDAlsoSet(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	var resolverCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/hub/sea-traces-api-key", r.URL.Path)
		atomic.AddInt32(&resolverCalls, 1)

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "sa-test", body["api_key"])
		require.Equal(t, serverURL(r), body["base_url"])
		require.Empty(t, body["project_id"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"project_id": "project-from-gateway",
			"base_url":   serverURL(r),
		})
	}))
	defer server.Close()

	client, err := NewClientWithConfig(context.Background(), Config{
		APIKey:    "sa-test",
		BaseURL:   server.URL,
		ProjectID: "project-should-not-short-circuit",
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, int32(1), atomic.LoadInt32(&resolverCalls))
}

func TestNewClientWithConfig_UsesCredentialsURLOverride(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	resolver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/credentials", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "sa-test", body["api_key"])
		require.Empty(t, body["project_id"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"project_id": "project-override",
			"base_url":   "https://resolver-response.example.com",
		})
	}))
	defer resolver.Close()

	client, err := NewClientWithConfig(
		context.Background(),
		Config{
			APIKey:  "sa-test",
			BaseURL: "https://gateway.example.com",
		},
		WithCredentialsURL(resolver.URL+"/credentials"),
	)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "https://resolver-response.example.com/api/public", client.restyCli.BaseURL)
}

func TestNewClientFromEnv_IncompleteDirectEnvUsesGatewayResponseOnly(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hub/sea-traces-api-key":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"project_id": "project-123",
				"base_url":   serverURL(r),
			})
		case "/api/public/health":
			require.Empty(t, r.Header.Get("Authorization"))
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

	t.Setenv("SEATRACES_PUBLIC_KEY", "pk-stale")
	t.Setenv("SEA_TRACES_API_KEY", "sa-test")
	t.Setenv("SEA_TRACES_BASE_URL", server.URL)

	client, err := NewClientFromEnv(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)

	_, err = client.Health().Check(context.Background())
	require.NoError(t, err)
}

func TestNewClientWithConfig_ProjectConfigCreatesNoAuthClient(t *testing.T) {
	client, err := NewClientWithConfig(context.Background(), Config{
		ProjectID: "project-123",
		BaseURL:   "https://internal.example.com",
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "https://internal.example.com/api/public", client.restyCli.BaseURL)
}

func TestNewClientWithConfig_MissingProjectBaseURLErrors(t *testing.T) {
	client, err := NewClientWithConfig(context.Background(), Config{
		ProjectID: "project-123",
	})
	require.ErrorIs(t, err, errMissingProjectConfig)
	require.Nil(t, client)
}

func TestNewClientWithConfig_ResolverFailureErrors(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClientWithConfig(context.Background(), Config{
		APIKey:  "sa-test",
		BaseURL: server.URL,
	})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "HTTP 401")
}

func TestNewClientWithConfig_ResolverMissingFieldErrors(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publicKey": "pk-resolved",
			"base_url":  "https://resolver-response.example.com",
		})
	}))
	defer server.Close()

	client, err := NewClientWithConfig(context.Background(), Config{
		APIKey:  "sa-test",
		BaseURL: server.URL,
	})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "invalid Sea Traces credentials response")
}

func TestNewClientWithConfig_MasksResolverRequestURLOnNetworkError(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	client, err := NewClientWithConfig(context.Background(), Config{
		APIKey:  "sa-super-secret-api-key",
		BaseURL: "https://sea-traces.example.invalid",
	})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "sa-sup...-key")
	require.NotContains(t, err.Error(), "sa-super-secret-api-key")
	require.NotContains(t, err.Error(), "secretKey")
}

func TestNewClientWithConfig_MasksCredentialsURLQueryOnResolverError(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClientWithConfig(
		context.Background(),
		Config{
			APIKey:  "sa-super-secret-api-key",
			BaseURL: "https://gateway.example.com",
		},
		WithCredentialsURL(server.URL+"/credentials?token=secret-token"),
	)
	require.Error(t, err)
	require.Nil(t, client)
	require.NotContains(t, err.Error(), "secret-token")
	require.NotContains(t, err.Error(), "?token=")
}

func TestNewClientWithConfig_DeduplicatesConcurrentResolution(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	var resolverCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&resolverCalls, 1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"project_id": "project-123",
			"base_url":   "https://resolver-response.example.com",
		})
	}))
	defer server.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := NewClientWithConfig(context.Background(), Config{
				APIKey:  "sa-test",
				BaseURL: server.URL,
			})
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

func TestNewClientFromEnv_GatewayEnv(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/hub/sea-traces-api-key", r.URL.Path)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "sa-env", body["api_key"])
		require.Empty(t, body["project_id"])
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"project_id": "project-env",
			"base_url":   "https://resolver-response.example.com",
		})
	}))
	defer server.Close()

	t.Setenv("SEA_TRACES_BASE_URL", server.URL)
	t.Setenv("SEA_TRACES_API_KEY", "sa-env")

	client, err := NewClientFromEnv(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "https://resolver-response.example.com/api/public", client.restyCli.BaseURL)
}

func TestNewClientFromEnv_LegacyDirectEnvFallback(t *testing.T) {
	clearSealangfuseCredentialsCacheForTest()

	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-legacy")
	t.Setenv("LANGFUSE_SECRET_KEY", "sk-legacy")
	t.Setenv("LANGFUSE_BASE_URL", "https://legacy.example.com")

	client, err := NewClientFromEnv(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "https://legacy.example.com/api/public", client.restyCli.BaseURL)
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

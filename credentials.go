package langfuse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	envSeaTeamKey             = "SEA_TEAM_KEY"
	envSeaTracesBaseURL       = "SEA_TRACES_BASE_URL"
	envSealangfuseCredentials = "SEALANGFUSE_CREDENTIALS_URL"
)

var (
	errMissingBaseURL = errors.New("SEA_TRACES_BASE_URL is required")
	errMissingAPIKey  = errors.New("SEA_TEAM_KEY is required")
)

type sealangfuseCredentials struct {
	PublicKey string `json:"publicKey"`
	SecretKey string `json:"secretKey"`
	BaseURL   string `json:"baseUrl"`
	Status    string `json:"status"`
}

type resolveSealangfuseCredentialsParams struct {
	apiKey         string
	baseURL        string
	credentialsURL string
	httpClient     *http.Client
}

type credentialsCall struct {
	done        chan struct{}
	credentials sealangfuseCredentials
	err         error
}

var credentialsResolverCache = struct {
	sync.Mutex
	values   map[string]sealangfuseCredentials
	inflight map[string]*credentialsCall
}{
	values:   make(map[string]sealangfuseCredentials),
	inflight: make(map[string]*credentialsCall),
}

func getSeaTeamKeyEnv() string {
	return strings.TrimSpace(os.Getenv(envSeaTeamKey))
}

func getSeaTracesBaseURLEnv() string {
	return strings.TrimSpace(os.Getenv(envSeaTracesBaseURL))
}

func getSealangfuseCredentialsURL(baseURL string, explicitCredentialsURL string) string {
	if explicitCredentialsURL != "" {
		return explicitCredentialsURL
	}
	if value := os.Getenv(envSealangfuseCredentials); value != "" {
		return value
	}

	return strings.TrimRight(baseURL, "/") + "/api/public/sea-project-api-credentials"
}

func resolveSealangfuseCredentials(ctx context.Context, params resolveSealangfuseCredentialsParams) (sealangfuseCredentials, error) {
	credentialsURL := getSealangfuseCredentialsURL(params.baseURL, params.credentialsURL)
	cacheKey := credentialsURL + "\n" + params.apiKey

	credentialsResolverCache.Lock()
	if credentials, ok := credentialsResolverCache.values[cacheKey]; ok {
		credentialsResolverCache.Unlock()
		return credentials, nil
	}
	if call, ok := credentialsResolverCache.inflight[cacheKey]; ok {
		credentialsResolverCache.Unlock()
		<-call.done
		return call.credentials, call.err
	}

	call := &credentialsCall{done: make(chan struct{})}
	credentialsResolverCache.inflight[cacheKey] = call
	credentialsResolverCache.Unlock()

	call.credentials, call.err = fetchSealangfuseCredentials(ctx, params, credentialsURL)

	credentialsResolverCache.Lock()
	if call.err == nil {
		credentialsResolverCache.values[cacheKey] = call.credentials
	}
	delete(credentialsResolverCache.inflight, cacheKey)
	close(call.done)
	credentialsResolverCache.Unlock()

	return call.credentials, call.err
}

func fetchSealangfuseCredentials(ctx context.Context, params resolveSealangfuseCredentialsParams, credentialsURL string) (sealangfuseCredentials, error) {
	parsedURL, err := url.Parse(credentialsURL)
	if err != nil {
		return sealangfuseCredentials{}, fmt.Errorf("invalid Sealangfuse credentials URL %q: %w", credentialsURL, err)
	}

	query := parsedURL.Query()
	query.Set("key", params.apiKey)
	parsedURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return sealangfuseCredentials{}, fmt.Errorf("create Sealangfuse credentials request: %w", err)
	}

	httpClient := params.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return sealangfuseCredentials{}, fmt.Errorf(
			"resolve Sealangfuse credentials from %s for key %s: %w",
			credentialsURL,
			maskSealangfuseAPIKey(params.apiKey),
			err,
		)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return sealangfuseCredentials{}, fmt.Errorf(
			"resolve Sealangfuse credentials from %s for key %s: HTTP %d",
			credentialsURL,
			maskSealangfuseAPIKey(params.apiKey),
			response.StatusCode,
		)
	}

	var credentials sealangfuseCredentials
	if err := json.NewDecoder(response.Body).Decode(&credentials); err != nil {
		return sealangfuseCredentials{}, fmt.Errorf("decode Sealangfuse credentials response: %w", err)
	}

	if credentials.Status != "ACTIVE" || credentials.PublicKey == "" || credentials.SecretKey == "" || credentials.BaseURL == "" {
		return sealangfuseCredentials{}, fmt.Errorf(
			"invalid Sealangfuse credentials response from %s for key %s",
			credentialsURL,
			maskSealangfuseAPIKey(params.apiKey),
		)
	}

	return credentials, nil
}

func maskSealangfuseAPIKey(apiKey string) string {
	if len(apiKey) <= 10 {
		return "<masked>"
	}

	return apiKey[:6] + "..." + apiKey[len(apiKey)-4:]
}

func clearSealangfuseCredentialsCacheForTest() {
	credentialsResolverCache.Lock()
	defer credentialsResolverCache.Unlock()

	credentialsResolverCache.values = make(map[string]sealangfuseCredentials)
	credentialsResolverCache.inflight = make(map[string]*credentialsCall)
}

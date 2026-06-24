package langfuse

import (
	"bytes"
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
	envSeaTracesPublicKey = "SEATRACES_PUBLIC_KEY"
	envSeaTracesSecretKey = "SEATRACES_SECRET_KEY"
	envSeaTracesBaseURL   = "SEATRACES_BASE_URL"
	envSeaTracesAPIKey    = "SEA_TRACES_API_KEY"
	envSeaTracesGateway   = "SEA_TRACES_BASE_URL"
	envSeaTracesProjectID = "SEA_TRACES_PROJECT_ID"

	envLegacyLangfusePublicKey = "LANGFUSE_PUBLIC_KEY"
	envLegacyLangfuseSecretKey = "LANGFUSE_SECRET_KEY"
	envLegacyLangfuseBaseURL   = "LANGFUSE_BASE_URL"
)

var (
	errMissingDirectConfig  = errors.New("SEATRACES_PUBLIC_KEY, SEATRACES_SECRET_KEY, and SEATRACES_BASE_URL are required for direct upload")
	errMissingGatewayConfig = errors.New("SEA_TRACES_API_KEY, SEA_TRACES_BASE_URL, and SEA_TRACES_PROJECT_ID are required for gateway auth")
)

type seaTracesCredentials struct {
	PublicKey string `json:"publicKey"`
	SecretKey string `json:"secretKey"`
	BaseURL   string `json:"baseUrl"`
}

type resolveSeaTracesCredentialsParams struct {
	apiKey         string
	baseURL        string
	projectID      string
	credentialsURL string
	httpClient     *http.Client
}

type credentialsCall struct {
	done        chan struct{}
	credentials seaTracesCredentials
	err         error
}

var credentialsResolverCache = struct {
	sync.Mutex
	values   map[string]seaTracesCredentials
	inflight map[string]*credentialsCall
}{
	values:   make(map[string]seaTracesCredentials),
	inflight: make(map[string]*credentialsCall),
}

func getSeaTracesPublicKeyEnv() string {
	return strings.TrimSpace(os.Getenv(envSeaTracesPublicKey))
}

func getSeaTracesSecretKeyEnv() string {
	return strings.TrimSpace(os.Getenv(envSeaTracesSecretKey))
}

func getSeaTracesBaseURLEnv() string {
	return strings.TrimSpace(os.Getenv(envSeaTracesBaseURL))
}

func getSeaTracesAPIKeyEnv() string {
	return strings.TrimSpace(os.Getenv(envSeaTracesAPIKey))
}

func getSeaTracesGatewayBaseURLEnv() string {
	return strings.TrimSpace(os.Getenv(envSeaTracesGateway))
}

func getSeaTracesProjectIDEnv() string {
	return strings.TrimSpace(os.Getenv(envSeaTracesProjectID))
}

func legacyDirectConfigFromEnv() Config {
	return Config{
		PublicKey: strings.TrimSpace(os.Getenv(envLegacyLangfusePublicKey)),
		SecretKey: strings.TrimSpace(os.Getenv(envLegacyLangfuseSecretKey)),
		BaseURL:   strings.TrimSpace(os.Getenv(envLegacyLangfuseBaseURL)),
	}
}

func getSeaTracesCredentialsURL(baseURL string, explicitCredentialsURL string) string {
	if explicitCredentialsURL != "" {
		return strings.TrimSpace(explicitCredentialsURL)
	}

	return strings.TrimRight(baseURL, "/") + "/hub/sea-traces-api-key"
}

func resolveSeaTracesCredentials(ctx context.Context, params resolveSeaTracesCredentialsParams) (seaTracesCredentials, error) {
	credentialsURL := getSeaTracesCredentialsURL(params.baseURL, params.credentialsURL)
	cacheKey := credentialsURL + "\n" + params.apiKey + "\n" + params.projectID

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

	call.credentials, call.err = fetchSeaTracesCredentials(ctx, params, credentialsURL)

	credentialsResolverCache.Lock()
	if call.err == nil {
		credentialsResolverCache.values[cacheKey] = call.credentials
	}
	delete(credentialsResolverCache.inflight, cacheKey)
	close(call.done)
	credentialsResolverCache.Unlock()

	return call.credentials, call.err
}

func fetchSeaTracesCredentials(ctx context.Context, params resolveSeaTracesCredentialsParams, credentialsURL string) (seaTracesCredentials, error) {
	if _, err := url.ParseRequestURI(credentialsURL); err != nil {
		return seaTracesCredentials{}, fmt.Errorf("invalid Sea Traces credentials URL %q: %w", maskSeaTracesCredentialsURL(credentialsURL), err)
	}

	body, err := json.Marshal(map[string]string{
		"api_key":    params.apiKey,
		"base_url":   params.baseURL,
		"project_id": params.projectID,
	})
	if err != nil {
		return seaTracesCredentials{}, fmt.Errorf("encode Sea Traces credentials request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, credentialsURL, bytes.NewReader(body))
	if err != nil {
		return seaTracesCredentials{}, fmt.Errorf("create Sea Traces credentials request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	httpClient := params.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return seaTracesCredentials{}, fmt.Errorf(
			"resolve Sea Traces credentials from %s for key %s: %v",
			maskSeaTracesCredentialsURL(credentialsURL),
			maskSeaTracesAPIKey(params.apiKey),
			maskSeaTracesRequestError(err),
		)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return seaTracesCredentials{}, fmt.Errorf(
			"resolve Sea Traces credentials from %s for key %s: HTTP %d",
			maskSeaTracesCredentialsURL(credentialsURL),
			maskSeaTracesAPIKey(params.apiKey),
			response.StatusCode,
		)
	}

	var credentials seaTracesCredentials
	if err := json.NewDecoder(response.Body).Decode(&credentials); err != nil {
		return seaTracesCredentials{}, fmt.Errorf("decode Sea Traces credentials response: %w", err)
	}

	if strings.TrimSpace(credentials.PublicKey) == "" || strings.TrimSpace(credentials.SecretKey) == "" || strings.TrimSpace(credentials.BaseURL) == "" {
		return seaTracesCredentials{}, fmt.Errorf(
			"invalid Sea Traces credentials response from %s for key %s",
			maskSeaTracesCredentialsURL(credentialsURL),
			maskSeaTracesAPIKey(params.apiKey),
		)
	}

	credentials.PublicKey = strings.TrimSpace(credentials.PublicKey)
	credentials.SecretKey = strings.TrimSpace(credentials.SecretKey)
	credentials.BaseURL = strings.TrimSpace(credentials.BaseURL)
	return credentials, nil
}

func maskSeaTracesAPIKey(apiKey string) string {
	if len(apiKey) <= 10 {
		return "<masked>"
	}

	return apiKey[:6] + "..." + apiKey[len(apiKey)-4:]
}

func maskSeaTracesRequestError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		masked := *urlErr
		if requestURL, parseErr := url.Parse(masked.URL); parseErr == nil {
			requestURL.RawQuery = ""
			masked.URL = requestURL.String()
		}
		return &masked
	}

	return err
}

func maskSeaTracesCredentialsURL(credentialsURL string) string {
	parsedURL, err := url.Parse(credentialsURL)
	if err != nil {
		return credentialsURL
	}
	parsedURL.User = nil
	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""
	return parsedURL.String()
}

func clearSealangfuseCredentialsCacheForTest() {
	credentialsResolverCache.Lock()
	defer credentialsResolverCache.Unlock()

	credentialsResolverCache.values = make(map[string]seaTracesCredentials)
	credentialsResolverCache.inflight = make(map[string]*credentialsCall)
}

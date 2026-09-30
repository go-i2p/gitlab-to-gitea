// client.go

// Package gitea provides a client for interacting with the Gitea API
package gitea

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Client handles communication with the Gitea API
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      string
}

// VersionResponse represents the Gitea version response
type VersionResponse struct {
	Version string `json:"version"`
}

// SearchResponse represents the structure of Gitea search responses
type SearchResponse struct {
	Data  []map[string]interface{} `json:"data"`
	OK    bool                     `json:"ok"`
	Total int                      `json:"total_count"`
}

// SearchRepositories searches for repositories and returns the results
func (c *Client) SearchRepositories() ([]map[string]interface{}, error) {
	var response SearchResponse
	err := c.Get("repos/search?limit=1000", &response)
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}

// FetchCSRFToken retrieves a CSRF token from Gitea
// I don't think it works.
func (c *Client) FetchCSRFToken() (string, error) {
	resp, err := c.request("GET", "/user/login", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to fetch login page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Use a single approach targeting Gitea's current HTML structure
	metaTagRegex := regexp.MustCompile(`<meta name="_csrf" content="([^"]+)"`)
	if matches := metaTagRegex.FindSubmatch(body); len(matches) > 1 {
		return string(matches[1]), nil
	}

	return "", fmt.Errorf("could not find CSRF token in login page")
}

// NewClient creates a Gitea API client. When insecureSkipTLSVerify is true,
// TLS certificates presented by the Gitea server are not verified.
func NewClient(baseURL, token string, insecureSkipTLSVerify bool) (*Client, error) {
	// Remove trailing slash from baseURL if present
	baseURL = strings.TrimSuffix(baseURL, "/")

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	return &Client{
		baseURL: u,
		httpClient: &http.Client{
			Timeout: 360 * time.Second,
			Transport: &http.Transport{
				Dial: Dial,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: insecureSkipTLSVerify, //nolint:gosec // opt-in via config
				},
			},
		},
		token: token,
	}, nil
}

// WithTimeout returns a copy of the client whose requests use the given
// timeout. A zero timeout disables the timeout entirely.
func (c *Client) WithTimeout(timeout time.Duration) *Client {
	httpClient := *c.httpClient
	httpClient.Timeout = timeout
	return &Client{
		baseURL:    c.baseURL,
		httpClient: &httpClient,
		token:      c.token,
	}
}

// Add a custom transport to handle CSRF tokens
type CSRFTokenTransport struct {
	Token     string
	CSRFToken string
	Base      http.RoundTripper
}

func (t *CSRFTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add Content-Type header
	req.Header.Set("Content-Type", "application/json")
	// Add authorization header
	req.Header.Set("Authorization", fmt.Sprintf("token %s", t.Token))
	// Add Accept header
	req.Header.Set("Accept", "application/json")
	// Add X-Gitea-CSRF header with the token to bypass CSRF protection
	req.Header.Set("X-Gitea-CSRF", t.CSRFToken)

	return t.Base.RoundTrip(req)
}

// GetVersion retrieves the Gitea version
func (c *Client) GetVersion() (string, error) {
	versionResp := &VersionResponse{}
	_, err := c.request("GET", "version", nil, versionResp)
	if err != nil {
		return "", err
	}
	return versionResp.Version, nil
}

// Get performs a GET request against the API
func (c *Client) Get(path string, result interface{}) error {
	_, err := c.request("GET", path, nil, result)
	return err
}

// Post performs a POST request against the API
func (c *Client) Post(path string, data, result interface{}) error {
	_, err := c.request("POST", path, data, result)
	return err
}

// Put performs a PUT request against the API
func (c *Client) Put(path string, data, result interface{}) error {
	_, err := c.request("PUT", path, data, result)
	return err
}

// Patch performs a PATCH request against the API
func (c *Client) Patch(path string, data, result interface{}) error {
	_, err := c.request("PATCH", path, data, result)
	return err
}

// Delete performs a DELETE request against the API
func (c *Client) Delete(path string) error {
	_, err := c.request("DELETE", path, nil, nil)
	return err
}

// request sends an HTTP request to the Gitea API
func (c *Client) request(method, path string, data, result interface{}) (*http.Response, error) {
	// Normalize path - remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	// Add API prefix if not already present
	if !strings.HasPrefix(path, "api/v1/") {
		path = "api/v1/" + path
	}

	// Construct the full URL without double slashes
	fullURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(c.baseURL.String(), "/"), path)

	// Debug output to see what endpoint is being called
	// utils.PrintInfo(fmt.Sprintf("Making %s request to: %s", method, fullURL))

	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("token %s", c.token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle error status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, fmt.Errorf("API returned error: %s - %s", resp.Status, string(bodyBytes))
	}

	if result != nil && len(bodyBytes) > 0 {
		if err := json.NewDecoder(bytes.NewBuffer(bodyBytes)).Decode(result); err != nil {
			return resp, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return resp, nil
}

// GetToken returns the authentication token
func (c *Client) GetToken() string {
	return c.token
}

// Copyright (c) 2024 OData MCP Contributors
// SPDX-License-Identifier: MIT

package client

import (
	"net/http"
	"strings"
	"time"

	"github.com/zmcp/odata-mcp/internal/constants"
)

// ODataClient handles HTTP communication with OData services
type ODataClient struct {
	baseURL        string
	httpClient     *http.Client
	cookies        map[string]string
	username       string
	password       string
	csrfToken      string
	verbose        bool
	sessionCookies []*http.Cookie // Track session cookies from server
	isV4           bool           // Whether the service is OData v4
}

// NewODataClient creates a new OData client
func NewODataClient(baseURL string, verbose bool) *ODataClient {
	// Ensure base URL ends with /
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	return &ODataClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(constants.DefaultTimeout) * time.Second,
		},
		verbose: verbose,
		isV4:    false, // Will be determined when fetching metadata
	}
}

// SetBasicAuth configures basic authentication
func (c *ODataClient) SetBasicAuth(username, password string) {
	c.username = username
	c.password = password
}

// SetCookies configures cookie authentication
func (c *ODataClient) SetCookies(cookies map[string]string) {
	c.cookies = cookies
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

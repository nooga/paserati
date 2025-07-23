package driver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"paserati/pkg/vm"
	"strings"
	"time"
)

// httpModule defines the paserati/http module with sync fetch functionality
func httpModule(m *ModuleBuilder) {
	// Export the main fetch function
	// The native module system passes variadic arguments as a slice, so we need to handle it differently
	m.Function("fetch", fetchWrapper)
	
	// Note: Response objects are created by fetch(), not directly constructible
	
	// Export Headers class - use map[string]interface{} directly, not variadic
	m.Class("Headers", (*Headers)(nil), func(init map[string]interface{}) *Headers {
		h := &Headers{
			headers: make(http.Header),
		}
		// Initialize with headers if provided
		for k, v := range init {
			if strVal, ok := v.(string); ok {
				h.headers.Set(k, strVal)
			}
		}
		return h
	})
}

// Headers represents the Headers API
type Headers struct {
	headers http.Header
}

// Methods for Headers
func (h *Headers) Get(name string) string {
	return h.headers.Get(name)
}

func (h *Headers) Has(name string) bool {
	return h.headers.Get(name) != ""
}

func (h *Headers) Set(name, value string) {
	h.headers.Set(name, value)
}

func (h *Headers) Delete(name string) {
	h.headers.Del(name)
}

// Response represents the Response API
type Response struct {
	OK         bool     `json:"ok"`
	Status     int      `json:"status"`
	StatusText string   `json:"statusText"`
	URL        string   `json:"url"`
	Headers    *Headers `json:"-"` // Not directly serialized
	body       []byte   // Internal body storage
	bodyUsed   bool
}

// Methods for Response
func (r *Response) Text() (string, error) {
	if r.bodyUsed {
		return "", fmt.Errorf("body already used")
	}
	r.bodyUsed = true
	return string(r.body), nil
}

func (r *Response) Json() (vm.Value, error) {
	if r.bodyUsed {
		return vm.Undefined, fmt.Errorf("body already used")
	}
	r.bodyUsed = true
	
	// Parse JSON directly into a vm.Value using the VM's JSON unmarshaler
	var result vm.Value
	if err := result.UnmarshalJSON(r.body); err != nil {
		return vm.Undefined, err
	}
	return result, nil
}

func (r *Response) Blob() ([]byte, error) {
	if r.bodyUsed {
		return nil, fmt.Errorf("body already used")
	}
	r.bodyUsed = true
	return r.body, nil
}

// FetchInit represents the init options for fetch (second parameter)
type FetchInit struct {
	Method  string      `json:"method"`
	Headers interface{} `json:"headers"` // Can be Headers object or plain object
	Body    interface{} `json:"body"`
}

// fetch performs a synchronous HTTP request mimicking the Fetch API
func fetch(url string, init interface{}) (*Response, error) {
	// Default options
	method := "GET"
	headers := &Headers{headers: make(http.Header)}
	var body io.Reader
	
	// Parse init options if provided
	if init != nil {
		if initMap, ok := init.(map[string]interface{}); ok {
			// Method
			if m, ok := initMap["method"].(string); ok {
				method = strings.ToUpper(m)
			}
			
			// Headers
			if h := initMap["headers"]; h != nil {
				switch v := h.(type) {
				case *Headers:
					headers = v
				case map[string]interface{}:
					for k, val := range v {
						if strVal, ok := val.(string); ok {
							headers.Set(k, strVal)
						}
					}
				}
			}
			
			// Body
			if b := initMap["body"]; b != nil {
				switch v := b.(type) {
				case string:
					body = strings.NewReader(v)
				case []byte:
					body = bytes.NewReader(v)
				default:
					// For other types (objects, etc.), check if we should auto-stringify
					contentType := headers.Get("Content-Type")
					if strings.Contains(strings.ToLower(contentType), "application/json") {
						// Auto-stringify objects for JSON content type using Go's JSON marshaler
						if jsonBytes, err := json.Marshal(v); err == nil {
							body = bytes.NewReader(jsonBytes)
						} else {
							return nil, fmt.Errorf("failed to serialize body to JSON: %w", err)
						}
					} else {
						return nil, fmt.Errorf("unsupported body type: %T (hint: use JSON.stringify() for objects or set Content-Type to application/json)", v)
					}
				}
			}
		}
	}
	
	// Create HTTP client with reasonable timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	// Create request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	
	
	// Set headers
	req.Header = headers.headers
	
	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	// Create response headers
	responseHeaders := &Headers{headers: resp.Header}
	
	// Create Response object
	response := &Response{
		OK:         resp.StatusCode >= 200 && resp.StatusCode < 300,
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		URL:        resp.Request.URL.String(),
		Headers:    responseHeaders,
		body:       bodyBytes,
		bodyUsed:   false,
	}
	
	return response, nil
}

// fetchWrapper handles the arguments properly for the native module system
func fetchWrapper(url string, init ...map[string]interface{}) (*Response, error) {
	var initMap map[string]interface{}
	if len(init) > 0 {
		initMap = init[0]
	}
	return fetch(url, initMap)
}
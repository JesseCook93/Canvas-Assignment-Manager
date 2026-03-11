package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"github.com/joho/godotenv"
)

// CanvasConfig holds Canvas API configuration
type CanvasConfig struct {
	BaseURL string
	Token   string
}

func main() {
	// Load .env file
	godotenv.Load()

	// Configuration
	baseURL := os.Getenv("CANVAS_URL")   
	token := os.Getenv("CANVAS_TOKEN")   // Canvas API token

	if baseURL == "" || token == "" {
		fmt.Println("Error: CANVAS_URL and CANVAS_TOKEN environment variables must be set")
		fmt.Println("Example:")
		fmt.Println("  $env:CANVAS_URL = 'https://canvas.instructure.com'")
		fmt.Println("  $env:CANVAS_TOKEN = 'your-api-token'")
		os.Exit(1)
	}

	config := CanvasConfig{
		BaseURL: baseURL,
		Token:   token,
	}

	// Example: Get current user info
	fmt.Println("=== Canvas API Client ===\n")
	userInfo, err := callCanvasAPI(config, "/api/v1/users/self")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Current User:")
	fmt.Println(userInfo)

	fmt.Println("\n---\n")

	// Example: Get list of courses
	courses, err := callCanvasAPI(config, "/api/v1/courses")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Courses:")
	fmt.Println(courses)
}

// callCanvasAPI makes an HTTP GET request to Canvas API with token authentication
func callCanvasAPI(config CanvasConfig, endpoint string) (string, error) {
	url := config.BaseURL + endpoint

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add Canvas authentication header (token in Authorization header)
	req.Header.Set("Authorization", "Bearer "+config.Token)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Canvas API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Pretty print JSON
	var jsonData interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		// If not JSON, return as-is
		return string(body), nil
	}

	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return string(body), nil
	}

	return string(prettyJSON), nil
}
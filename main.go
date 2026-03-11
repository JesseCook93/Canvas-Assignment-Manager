package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/joho/godotenv"
)

// CanvasConfig holds Canvas API configuration
type CanvasConfig struct {
	BaseURL string
	Token   string
}

// Course represents a Canvas course
type Course struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	WorkflowState string `json:"workflow_state"`
}

// Submission represents a Canvas submission
type Submission struct {
	ID            int    `json:"id"`
	AssignmentID  int    `json:"assignment_id"`
	UserID        int    `json:"user_id"`
	SubmittedAt   string `json:"submitted_at"`
	WorkflowState string `json:"workflow_state"`
}

// Assignment represents a Canvas assignment
type Assignment struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	DueAt         string `json:"due_at"`
	CourseID      int    `json:"course_id"`
	CourseName    string `json:"-"`
	HasSubmission bool   `json:"-"`
}

func main() {
	// Parse command line flags
	submit := flag.Bool("submit", false, "enable submission mode to submit assignments")
	flag.Parse()

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

	// Get all courses
	fmt.Println("==================================")
	fmt.Println("=== Canvas Assignments Manager ===")
	fmt.Println("==================================")
	fmt.Println()
	courses, err := getCourses(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(courses) == 0 {
		fmt.Println("No available courses found.")
		return
	}

	// Get assignments from all available courses
	var allAssignments []Assignment
	for _, course := range courses {
		assignments, err := getAssignments(config, course.ID)
		if err != nil {
			fmt.Printf("Warning: Error getting assignments for course %s: %v\n", course.Name, err)
			continue
		}

		for _, assignment := range assignments {
			assignment.CourseID = course.ID
			assignment.CourseName = course.Name
			// Check if assignment has a submission
			hasSubmission, err := checkSubmission(config, course.ID, assignment.ID)
			if err != nil {
				fmt.Printf("Warning: Error checking submission for %s: %v\n", assignment.Name, err)
			}
			assignment.HasSubmission = hasSubmission
			allAssignments = append(allAssignments, assignment)
		}
	}

	if len(allAssignments) == 0 {
		fmt.Println("No assignments found.")
		return
	}

	// Filter assignments to only those due today or in the future
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	
	var filteredAssignments []Assignment
	for _, assignment := range allAssignments {
		// Skip if already has a submission
		if assignment.HasSubmission {
			continue
		}
		if assignment.DueAt == "" {
			continue
		}
		dueTime, err := time.Parse(time.RFC3339, assignment.DueAt)
		if err != nil {
			continue
		}
		// Include if due date is today or in the future
		if dueTime.After(todayStart) || isSameDay(dueTime, now) {
			filteredAssignments = append(filteredAssignments, assignment)
		}
	}

	if len(filteredAssignments) == 0 {
		fmt.Println("No upcoming assignments found.")
		return
	}

	// Sort assignments by due date (closest to today first)
	sort.Slice(filteredAssignments, func(i, j int) bool {
		timeI, errI := time.Parse(time.RFC3339, filteredAssignments[i].DueAt)
		timeJ, errJ := time.Parse(time.RFC3339, filteredAssignments[j].DueAt)

		// If either fails to parse, put the valid one first
		if errI != nil {
			return false
		}
		if errJ != nil {
			return true
		}

		return timeI.Before(timeJ)
	})

	// Handle submission mode or display mode
	if *submit {
		handleSubmission(config, filteredAssignments)
	} else {
		displayAssignments(filteredAssignments)
	}
}

// displayAssignments shows all upcoming assignments in a table
func displayAssignments(assignments []Assignment) {
	fmt.Printf("Found %d upcoming assignments:\n\n", len(assignments))
	
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ASSIGNMENT NAME\tCOURSE NAME\tDUE DATE")
	fmt.Fprintln(w, "----------------\t-----------\t--------")
	
	for _, assignment := range assignments {
		dueDate := "No due date"
		if assignment.DueAt != "" {
			// Parse and format the date more readably
			if parsedTime, err := time.Parse(time.RFC3339, assignment.DueAt); err == nil {
				dueDate = parsedTime.Format("Jan 02, 2006 3:04 PM")
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", assignment.Name, assignment.CourseName, dueDate)
	}
	w.Flush()
}

// handleSubmission allows user to select and submit an assignment
func handleSubmission(config CanvasConfig, assignments []Assignment) {
	fmt.Printf("Found %d upcoming assignments:\n\n", len(assignments))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tASSIGNMENT NAME\tCOURSE NAME\tDUE DATE")
	fmt.Fprintln(w, "-\t---------------\t-----------\t--------")

	for i, assignment := range assignments {
		dueDate := "No due date"
		if assignment.DueAt != "" {
			if parsedTime, err := time.Parse(time.RFC3339, assignment.DueAt); err == nil {
				dueDate = parsedTime.Format("Jan 02, 2006 3:04 PM")
			}
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", i+1, assignment.Name, assignment.CourseName, dueDate)
	}
	w.Flush()
	fmt.Println()

	// Get user selection
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Select assignment to submit (number): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	index, err := strconv.Atoi(input)
	if err != nil || index < 1 || index > len(assignments) {
		fmt.Println("Invalid selection.")
		return
	}

	selected := assignments[index-1]

	// Get submission type
	fmt.Println("\nSubmission types:")
	fmt.Println("1. Text Entry")
	fmt.Println("2. File Upload")
	fmt.Println("3. URL")
	fmt.Print("Choose submission type (1-3): ")
	typeInput, _ := reader.ReadString('\n')
	typeInput = strings.TrimSpace(typeInput)

	var submissionType string
	var submissionBody string

	switch typeInput {
	case "1":
		fmt.Print("Enter your submission text: ")
		text, _ := reader.ReadString('\n')
		submissionType = "online_text_entry"
		submissionBody = strings.TrimSpace(text)

	case "2":
		fmt.Print("Enter file path: ")
		filePath, _ := reader.ReadString('\n')
		filePath = strings.TrimSpace(filePath)
		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			return
		}
		submissionType = "online_upload"
		submissionBody = string(fileContent)

	case "3":
		fmt.Print("Enter submission URL: ")
		urlInput, _ := reader.ReadString('\n')
		submissionType = "online_url"
		submissionBody = strings.TrimSpace(urlInput)

	default:
		fmt.Println("Invalid submission type.")
		return
	}

	// Submit assignment
	err = submitAssignment(config, selected.CourseID, selected.ID, submissionType, submissionBody)
	if err != nil {
		fmt.Printf("Error submitting assignment: %v\n", err)
		return
	}

	fmt.Printf("✓ Successfully submitted \"%s\" for %s\n", selected.Name, selected.CourseName)
}

// submitAssignment submits an assignment to Canvas
func submitAssignment(config CanvasConfig, courseID, assignmentID int, submissionType, body string) error {
	endpoint := fmt.Sprintf("/api/v1/courses/%d/assignments/%d/submissions", courseID, assignmentID)
	apiURL := config.BaseURL + endpoint

	data := url.Values{}
	data.Set("submission[submission_type]", submissionType)

	if submissionType == "online_text_entry" {
		data.Set("submission[body]", body)
	} else if submissionType == "online_url" {
		data.Set("submission[url]", body)
	} else if submissionType == "online_upload" {
		// For file uploads, we'd need multipart/form-data handling
		// For now, we'll use the text body approach
		data.Set("submission[body]", body)
	}

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+config.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Canvas API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil}

// getCourses fetches all favorited courses for the current user
func getCourses(config CanvasConfig) ([]Course, error) {
	endpoint := "/api/v1/users/self/favorites/courses"
	url := config.BaseURL + endpoint

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+config.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Canvas API returned status %d: %s", resp.StatusCode, string(body))
	}

	var courses []Course
	if err := json.Unmarshal(body, &courses); err != nil {
		return nil, fmt.Errorf("failed to parse courses: %w", err)
	}

	return courses, nil
}

// getAssignments fetches all assignments for a specific course, handling pagination
func getAssignments(config CanvasConfig, courseID int) ([]Assignment, error) {
	var allAssignments []Assignment
	endpoint := fmt.Sprintf("/api/v1/courses/%d/assignments?per_page=100", courseID)
	url := config.BaseURL + endpoint

	for {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+config.Token)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("Canvas API returned status %d: %s", resp.StatusCode, string(body))
		}

		var assignments []Assignment
		if err := json.Unmarshal(body, &assignments); err != nil {
			return nil, fmt.Errorf("failed to parse assignments: %w", err)
		}

		allAssignments = append(allAssignments, assignments...)

		// Check for next page in Link header
		linkHeader := resp.Header.Get("Link")
		nextURL := extractNextLink(linkHeader)
		if nextURL == "" {
			break
		}
		url = nextURL
	}

	return allAssignments, nil
}

// extractNextLink extracts the "next" URL from a Link header
func extractNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	// Link header format: <url>; rel="next", <url>; rel="last"
	links := parseLinkHeader(linkHeader)
	return links["next"]
}

// parseLinkHeader parses the Link header and returns a map of rel -> url
func parseLinkHeader(linkHeader string) map[string]string {
	links := make(map[string]string)
	parts := split(linkHeader, ",")

	for _, part := range parts {
		part = trimSpace(part)
		if part == "" {
			continue
		}

		// Split on semicolon to separate URL from rel
		pieces := split(part, ";")
		if len(pieces) < 2 {
			continue
		}

		url := trimSpace(pieces[0])
		if len(url) >= 2 && url[0] == '<' && url[len(url)-1] == '>' {
			url = url[1 : len(url)-1]
		}

		rel := ""
		for i := 1; i < len(pieces); i++ {
			p := trimSpace(pieces[i])
			if len(p) > 5 && p[:5] == "rel=\"" {
				rel = p[5 : len(p)-1]
				break
			}
		}

		if rel != "" {
			links[rel] = url
		}
	}

	return links
}

// Helper function to split strings
func split(s, sep string) []string {
	var parts []string
	var current string
	for _, c := range s {
		if string(c) == sep {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	parts = append(parts, current)
	return parts
}

// Helper function to trim whitespace
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// isSameDay checks if two times are on the same day
func isSameDay(t1, t2 time.Time) bool {
	return t1.Year() == t2.Year() && t1.YearDay() == t2.YearDay()
}

// checkSubmission checks if a user has submitted an assignment
func checkSubmission(config CanvasConfig, courseID, assignmentID int) (bool, error) {
	endpoint := fmt.Sprintf("/api/v1/courses/%d/assignments/%d/submissions/self", courseID, assignmentID)
	apiURL := config.BaseURL + endpoint

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+config.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check if there's a 404 (no submission)
	if resp.StatusCode == 404 {
		return false, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("Canvas API returned status %d: %s", resp.StatusCode, string(body))
	}

	var submission Submission
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, &submission); err != nil {
		return false, fmt.Errorf("failed to parse submission: %w", err)
	}

	// A submission exists if it has an ID and the workflow state is not "unsubmitted"
	return submission.WorkflowState != "unsubmitted", nil
}
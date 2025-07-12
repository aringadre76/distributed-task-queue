package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"distributed-task-queue/pkg/types"
	"github.com/spf13/cobra"
)

var (
	apiBaseURL string
	verbose    bool
	timeout    int
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "dtq-cli",
		Short: "Distributed Task Queue CLI",
		Long:  "Command-line interface for managing the Distributed Task Queue system",
	}

	rootCmd.PersistentFlags().StringVar(&apiBaseURL, "api-url", "http://localhost:8080", "API server base URL")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 30, "Request timeout in seconds")

	rootCmd.AddCommand(
		createTaskCommands(),
		createWorkerCommands(),
		createQueueCommands(),
		createSystemCommands(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func createTaskCommands() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Task management commands",
	}

	submitCmd := &cobra.Command{
		Use:   "submit [type] [payload]",
		Short: "Submit a new task",
		Args:  cobra.ExactArgs(2),
		RunE:  submitTask,
	}
	submitCmd.Flags().String("priority", "medium", "Task priority (high, medium, low)")
	submitCmd.Flags().Int("max-retries", 3, "Maximum retry attempts")
	submitCmd.Flags().Int("timeout", 300, "Task timeout in seconds")
	submitCmd.Flags().String("scheduled-at", "", "Schedule task for later (RFC3339 format)")

	statusCmd := &cobra.Command{
		Use:   "status [task-id]",
		Short: "Get task status",
		Args:  cobra.ExactArgs(1),
		RunE:  getTaskStatus,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE:  listTasks,
	}
	listCmd.Flags().String("status", "", "Filter by status")
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("size", 10, "Page size")

	cancelCmd := &cobra.Command{
		Use:   "cancel [task-id]",
		Short: "Cancel a task",
		Args:  cobra.ExactArgs(1),
		RunE:  cancelTask,
	}

	taskCmd.AddCommand(submitCmd, statusCmd, listCmd, cancelCmd)
	return taskCmd
}

func createWorkerCommands() *cobra.Command {
	workerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Worker management commands",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List workers",
		RunE:  listWorkers,
	}

	statsCmd := &cobra.Command{
		Use:   "stats [worker-id]",
		Short: "Get worker statistics",
		Args:  cobra.ExactArgs(1),
		RunE:  getWorkerStats,
	}

	workerCmd.AddCommand(listCmd, statsCmd)
	return workerCmd
}

func createQueueCommands() *cobra.Command {
	queueCmd := &cobra.Command{
		Use:   "queue",
		Short: "Queue management commands",
	}

	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Get queue statistics",
		RunE:  getQueueStats,
	}

	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check queue health",
		RunE:  checkQueueHealth,
	}

	queueCmd.AddCommand(statsCmd, healthCmd)
	return queueCmd
}

func createSystemCommands() *cobra.Command {
	systemCmd := &cobra.Command{
		Use:   "system",
		Short: "System management commands",
	}

	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check system health",
		RunE:  checkSystemHealth,
	}

	metricsCmd := &cobra.Command{
		Use:   "metrics",
		Short: "Get system metrics",
		RunE:  getMetrics,
	}

	systemCmd.AddCommand(healthCmd, metricsCmd)
	return systemCmd
}

func submitTask(cmd *cobra.Command, args []string) error {
	taskType := args[0]
	payloadStr := args[1]

	priority, _ := cmd.Flags().GetString("priority")
	maxRetries, _ := cmd.Flags().GetInt("max-retries")
	timeoutSecs, _ := cmd.Flags().GetInt("timeout")
	scheduledAtStr, _ := cmd.Flags().GetString("scheduled-at")

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return fmt.Errorf("invalid JSON payload: %w", err)
	}

	priorityEnum, err := parsePriority(priority)
	if err != nil {
		return err
	}

	request := map[string]interface{}{
		"type":        taskType,
		"payload":     payload,
		"priority":    priorityEnum,
		"max_retries": maxRetries,
		"timeout":     timeoutSecs,
	}

	if scheduledAtStr != "" {
		scheduledAt, err := time.Parse(time.RFC3339, scheduledAtStr)
		if err != nil {
			return fmt.Errorf("invalid scheduled_at format: %w", err)
		}
		request["scheduled_at"] = scheduledAt
	}

	response, err := makeAPIRequest("POST", "/api/v1/tasks", request)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Response: %s\n", response)
	} else {
		var task map[string]interface{}
		if err := json.Unmarshal(response, &task); err == nil {
			fmt.Printf("Task submitted successfully: %s\n", task["id"])
		} else {
			fmt.Printf("Task submitted: %s\n", response)
		}
	}

	return nil
}

func getTaskStatus(cmd *cobra.Command, args []string) error {
	taskID := args[0]

	response, err := makeAPIRequest("GET", "/api/v1/tasks/"+taskID, nil)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Response: %s\n", response)
	} else {
		var task map[string]interface{}
		if err := json.Unmarshal(response, &task); err == nil {
			fmt.Printf("Task ID: %s\n", task["id"])
			fmt.Printf("Type: %s\n", task["type"])
			fmt.Printf("Status: %s\n", task["status"])
			fmt.Printf("Priority: %s\n", task["priority"])
			fmt.Printf("Created: %s\n", task["created_at"])
			if workerID, ok := task["worker_id"]; ok && workerID != nil {
				fmt.Printf("Worker: %s\n", workerID)
			}
			if startedAt, ok := task["started_at"]; ok && startedAt != nil {
				fmt.Printf("Started: %s\n", startedAt)
			}
			if completedAt, ok := task["completed_at"]; ok && completedAt != nil {
				fmt.Printf("Completed: %s\n", completedAt)
			}
		} else {
			fmt.Printf("Task: %s\n", response)
		}
	}

	return nil
}

func listTasks(cmd *cobra.Command, args []string) error {
	status, _ := cmd.Flags().GetString("status")
	page, _ := cmd.Flags().GetInt("page")
	size, _ := cmd.Flags().GetInt("size")

	url := fmt.Sprintf("/api/v1/tasks?page=%d&size=%d", page, size)
	if status != "" {
		url += "&status=" + status
	}

	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Response: %s\n", response)
	} else {
		var result map[string]interface{}
		if err := json.Unmarshal(response, &result); err == nil {
			if tasks, ok := result["tasks"].([]interface{}); ok {
				fmt.Printf("Found %d tasks:\n\n", len(tasks))
				for _, t := range tasks {
					if task, ok := t.(map[string]interface{}); ok {
						fmt.Printf("ID: %s | Type: %s | Status: %s | Priority: %s\n",
							task["id"], task["type"], task["status"], task["priority"])
					}
				}
			}
		} else {
			fmt.Printf("Tasks: %s\n", response)
		}
	}

	return nil
}

func cancelTask(cmd *cobra.Command, args []string) error {
	taskID := args[0]

	response, err := makeAPIRequest("DELETE", "/api/v1/tasks/"+taskID, nil)
	if err != nil {
		return err
	}

	fmt.Printf("Task %s cancelled successfully\n", taskID)
	if verbose {
		fmt.Printf("Response: %s\n", response)
	}

	return nil
}

func listWorkers(cmd *cobra.Command, args []string) error {
	response, err := makeAPIRequest("GET", "/api/v1/workers", nil)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Response: %s\n", response)
	} else {
		var workers []map[string]interface{}
		if err := json.Unmarshal(response, &workers); err == nil {
			fmt.Printf("Found %d workers:\n\n", len(workers))
			for _, worker := range workers {
				fmt.Printf("ID: %s | Status: %s | Tasks: %.0f | Last Seen: %s\n",
					worker["id"], worker["status"], worker["tasks_handled"], worker["last_seen"])
			}
		} else {
			fmt.Printf("Workers: %s\n", response)
		}
	}

	return nil
}

func getWorkerStats(cmd *cobra.Command, args []string) error {
	workerID := args[0]

	response, err := makeAPIRequest("GET", "/api/v1/workers/"+workerID+"/stats", nil)
	if err != nil {
		return err
	}

	fmt.Printf("Worker %s statistics:\n%s\n", workerID, response)
	return nil
}

func getQueueStats(cmd *cobra.Command, args []string) error {
	response, err := makeAPIRequest("GET", "/api/v1/queue/stats", nil)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Response: %s\n", response)
	} else {
		var stats map[string]interface{}
		if err := json.Unmarshal(response, &stats); err == nil {
			fmt.Printf("Queue Statistics:\n")
			fmt.Printf("High Priority: %.0f\n", stats["high_priority_count"])
			fmt.Printf("Medium Priority: %.0f\n", stats["medium_priority_count"])
			fmt.Printf("Low Priority: %.0f\n", stats["low_priority_count"])
			fmt.Printf("Processing: %.0f\n", stats["processing_count"])
			fmt.Printf("Dead Letter: %.0f\n", stats["dead_letter_count"])
		} else {
			fmt.Printf("Queue Stats: %s\n", response)
		}
	}

	return nil
}

func checkQueueHealth(cmd *cobra.Command, args []string) error {
	response, err := makeAPIRequest("GET", "/api/v1/queue/health", nil)
	if err != nil {
		return err
	}

	fmt.Printf("Queue Health: %s\n", response)
	return nil
}

func checkSystemHealth(cmd *cobra.Command, args []string) error {
	response, err := makeAPIRequest("GET", "/api/v1/health", nil)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Response: %s\n", response)
	} else {
		var health map[string]interface{}
		if err := json.Unmarshal(response, &health); err == nil {
			fmt.Printf("System Health: %s\n", health["status"])
			if checks, ok := health["checks"].(map[string]interface{}); ok {
				for name, status := range checks {
					fmt.Printf("  %s: %s\n", name, status)
				}
			}
		} else {
			fmt.Printf("System Health: %s\n", response)
		}
	}

	return nil
}

func getMetrics(cmd *cobra.Command, args []string) error {
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Get(apiBaseURL + ":9090/metrics")
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metrics endpoint returned status: %d", resp.StatusCode)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Printf("System Metrics:\n%s", buf.String())

	return nil
}

func makeAPIRequest(method, endpoint string, data interface{}) ([]byte, error) {
	var body []byte
	var err error

	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request data: %w", err)
		}
	}

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	req, err := http.NewRequest(method, apiBaseURL+endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	respBody := new(bytes.Buffer)
	respBody.ReadFrom(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, respBody.String())
	}

	return respBody.Bytes(), nil
}

func parsePriority(priority string) (types.TaskPriority, error) {
	switch strings.ToLower(priority) {
	case "high":
		return types.PriorityHigh, nil
	case "medium":
		return types.PriorityMedium, nil
	case "low":
		return types.PriorityLow, nil
	default:
		return "", fmt.Errorf("invalid priority: %s", priority)
	}
} 
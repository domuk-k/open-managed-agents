package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage sessions",
}

var sessionsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new session",
	RunE: func(cmd *cobra.Command, args []string) error {
		agentID, _ := cmd.Flags().GetString("agent")
		envID, _ := cmd.Flags().GetString("env")
		title, _ := cmd.Flags().GetString("title")

		body := map[string]interface{}{
			"agent_id":       agentID,
			"environment_id": envID,
		}
		if title != "" {
			body["title"] = title
		}

		data, err := json.Marshal(body)
		if err != nil {
			return err
		}

		c := newClient()
		resp, err := c.post("/v1/sessions", data)
		if err != nil {
			return err
		}

		fmt.Println(string(resp))
		return nil
	},
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		resp, err := c.get("/v1/sessions")
		if err != nil {
			return err
		}

		var sessions []struct {
			ID            string  `json:"id"`
			Agent         string  `json:"agent"`
			EnvironmentID string  `json:"environment_id"`
			Title         *string `json:"title"`
			Status        string  `json:"status"`
		}
		if err := json.Unmarshal(resp, &sessions); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		if len(sessions) == 0 {
			fmt.Println("No sessions found.")
			return nil
		}

		fmt.Printf("%-36s\t%-36s\t%-36s\t%-20s\t%s\n", "ID", "AGENT", "ENVIRONMENT", "TITLE", "STATUS")
		for _, s := range sessions {
			title := ""
			if s.Title != nil {
				title = *s.Title
			}
			fmt.Printf("%-36s\t%-36s\t%-36s\t%-20s\t%s\n", s.ID, s.Agent, s.EnvironmentID, title, s.Status)
		}
		return nil
	},
}

var sessionsStreamCmd = &cobra.Command{
	Use:   "stream [id]",
	Short: "Stream session events via SSE",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := args[0]
		c := newClient()

		resp, err := c.getStream("/v1/sessions/" + sessionID + "/stream")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		fmt.Printf("Streaming events for session %s (Ctrl+C to stop)...\n", sessionID)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				fmt.Println(data)
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("stream error: %w", err)
		}
		return nil
	},
}

func init() {
	sessionsCreateCmd.Flags().String("agent", "", "Agent ID")
	sessionsCreateCmd.Flags().String("env", "", "Environment ID")
	sessionsCreateCmd.Flags().String("title", "", "Session title")
	sessionsCmd.AddCommand(sessionsCreateCmd)
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsStreamCmd)
}

package cli

import (
	"fmt"

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
		fmt.Printf("Creating session: agent=%s env=%s\n", agentID, envID)
		return nil
	},
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Listing sessions...")
		return nil
	},
}

var sessionsStreamCmd = &cobra.Command{
	Use:   "stream [id]",
	Short: "Stream session events via SSE",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Streaming session: %s\n", args[0])
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

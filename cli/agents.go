package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage agents",
}

var agentsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		model, _ := cmd.Flags().GetString("model")
		fmt.Printf("Creating agent: name=%s model=%s\n", name, model)
		return nil
	},
}

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Listing agents...")
		return nil
	},
}

var agentsGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get agent details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Agent: %s\n", args[0])
		return nil
	},
}

func init() {
	agentsCreateCmd.Flags().String("name", "", "Agent name")
	agentsCreateCmd.Flags().String("model", "", "Model ID (e.g. openai/gpt-4o)")
	agentsCreateCmd.Flags().String("system", "", "System prompt")
	agentsCmd.AddCommand(agentsCreateCmd)
	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsGetCmd)
}

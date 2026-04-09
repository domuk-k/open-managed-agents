package cli

import (
	"encoding/json"
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
		system, _ := cmd.Flags().GetString("system")

		body := map[string]interface{}{
			"name": name,
			"model": map[string]string{
				"id": model,
			},
		}
		if system != "" {
			body["system"] = system
		}

		data, err := json.Marshal(body)
		if err != nil {
			return err
		}

		c := newClient()
		resp, err := c.post("/v1/agents", data)
		if err != nil {
			return err
		}

		fmt.Println(string(resp))
		return nil
	},
}

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		resp, err := c.get("/v1/agents")
		if err != nil {
			return err
		}

		var agents []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Model   struct {
				ID string `json:"id"`
			} `json:"model"`
			Version int `json:"version"`
		}
		if err := json.Unmarshal(resp, &agents); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		if len(agents) == 0 {
			fmt.Println("No agents found.")
			return nil
		}

		fmt.Printf("%-36s\t%-20s\t%-30s\t%s\n", "ID", "NAME", "MODEL", "VERSION")
		for _, a := range agents {
			fmt.Printf("%-36s\t%-20s\t%-30s\t%d\n", a.ID, a.Name, a.Model.ID, a.Version)
		}
		return nil
	},
}

var agentsGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get agent details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		resp, err := c.get("/v1/agents/" + args[0])
		if err != nil {
			return err
		}

		// Pretty-print JSON
		var out json.RawMessage
		if err := json.Unmarshal(resp, &out); err != nil {
			fmt.Println(string(resp))
			return nil
		}
		pretty, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(pretty))
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

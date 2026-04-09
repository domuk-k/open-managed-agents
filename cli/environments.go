package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var environmentsCmd = &cobra.Command{
	Use:     "environments",
	Aliases: []string{"env"},
	Short:   "Manage environments",
}

var envsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		envType, _ := cmd.Flags().GetString("type")
		networking, _ := cmd.Flags().GetString("networking")

		body := map[string]interface{}{
			"name": name,
			"config": map[string]interface{}{
				"type": envType,
				"networking": map[string]string{
					"type": networking,
				},
			},
		}

		data, err := json.Marshal(body)
		if err != nil {
			return err
		}

		c := newClient()
		resp, err := c.post("/v1/environments", data)
		if err != nil {
			return err
		}

		fmt.Println(string(resp))
		return nil
	},
}

var envsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		resp, err := c.get("/v1/environments")
		if err != nil {
			return err
		}

		var envs []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Config struct {
				Type       string `json:"type"`
				Networking struct {
					Type string `json:"type"`
				} `json:"networking"`
			} `json:"config"`
		}
		if err := json.Unmarshal(resp, &envs); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		if len(envs) == 0 {
			fmt.Println("No environments found.")
			return nil
		}

		fmt.Printf("%-36s\t%-20s\t%-10s\t%s\n", "ID", "NAME", "TYPE", "NETWORKING")
		for _, e := range envs {
			fmt.Printf("%-36s\t%-20s\t%-10s\t%s\n", e.ID, e.Name, e.Config.Type, e.Config.Networking.Type)
		}
		return nil
	},
}

func init() {
	envsCreateCmd.Flags().String("name", "", "Environment name")
	envsCreateCmd.Flags().String("type", "docker", "Environment type (docker|local)")
	envsCreateCmd.Flags().String("networking", "unrestricted", "Network mode")
	environmentsCmd.AddCommand(envsCreateCmd)
	environmentsCmd.AddCommand(envsListCmd)
}

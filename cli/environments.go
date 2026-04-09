package cli

import (
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
		fmt.Printf("Creating environment: name=%s type=%s\n", name, envType)
		return nil
	},
}

var envsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Listing environments...")
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

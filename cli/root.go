package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "oma",
	Short: "Open Managed Agents - self-hosted agent platform",
	Long:  "Open-source Claude Managed Agents clone. Provider-agnostic, self-hosted, single binary.",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(agentsCmd)
	rootCmd.AddCommand(environmentsCmd)
	rootCmd.AddCommand(sessionsCmd)
}

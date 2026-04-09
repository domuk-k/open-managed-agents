package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "dev"

// SetVersion sets the version string displayed by the --version flag.
func SetVersion(v string) {
	version = v
}

var rootCmd = &cobra.Command{
	Use:   "oma",
	Short: "Open Managed Agents - self-hosted agent platform",
	Long:  "Open-source Claude Managed Agents clone. Provider-agnostic, self-hosted, single binary.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of OMA",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("OMA (Open Managed Agents) %s\n", version)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(agentsCmd)
	rootCmd.AddCommand(environmentsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(versionCmd)
}

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage the OMA server",
}

var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the OMA server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetString("port")
		fmt.Printf("Starting OMA server on :%s\n", port)
		// TODO: initialize store, create server, start
		return nil
	},
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the OMA server",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Stopping OMA server...")
		return nil
	},
}

func init() {
	serverStartCmd.Flags().StringP("port", "p", "8080", "Port to listen on")
	serverStartCmd.Flags().String("db", "./data/oma.db", "Database path")
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
}

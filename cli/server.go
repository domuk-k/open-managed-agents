package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/domuk-k/open-managed-agents/internal/api"
	"github.com/domuk-k/open-managed-agents/internal/config"
	"github.com/domuk-k/open-managed-agents/internal/store"
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
		cfg := config.Load()

		// Override from flags if explicitly set
		if cmd.Flags().Changed("port") {
			port, _ := cmd.Flags().GetString("port")
			cfg.Port = port
		}
		if cmd.Flags().Changed("db") {
			db, _ := cmd.Flags().GetString("db")
			cfg.DBPath = db
		}

		// Create data directory if needed
		dbDir := filepath.Dir(cfg.DBPath)
		if err := os.MkdirAll(dbDir, 0o755); err != nil {
			return fmt.Errorf("create data directory %s: %w", dbDir, err)
		}

		// Open SQLite store
		s, err := store.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}

		// Create API server
		srv := api.NewServer(cfg, s)

		// Handle graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\nShutting down server...")
			if err := srv.Shutdown(); err != nil {
				fmt.Fprintf(os.Stderr, "shutdown error: %v\n", err)
			}
		}()

		addr := ":" + cfg.Port
		fmt.Printf("Starting OMA server on %s\n", addr)
		if err := srv.Start(addr); err != nil {
			// echo returns http.ErrServerClosed on graceful shutdown
			if err.Error() == "http: Server closed" {
				fmt.Println("Server stopped.")
				return nil
			}
			return fmt.Errorf("server error: %w", err)
		}
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

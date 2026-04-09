package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

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
		defer s.Close()

		// Create API server
		srv := api.NewServer(cfg, s)

		// Write PID file
		pidDir := filepath.Join(os.Getenv("HOME"), ".oma")
		if err := os.MkdirAll(pidDir, 0o755); err != nil {
			return fmt.Errorf("create pid directory: %w", err)
		}
		pidFile := filepath.Join(pidDir, "oma.pid")
		if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644); err != nil {
			return fmt.Errorf("write pid file: %w", err)
		}
		defer os.Remove(pidFile)

		// Handle graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\nShutting down server...")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
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
		pidFile := filepath.Join(os.Getenv("HOME"), ".oma", "oma.pid")
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return fmt.Errorf("read pid file: %w (is the server running?)", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return fmt.Errorf("parse pid: %w", err)
		}
		fmt.Printf("Stopping OMA server (PID %d)...\n", pid)
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			return fmt.Errorf("send SIGTERM to PID %d: %w", pid, err)
		}
		// Clean up PID file
		os.Remove(pidFile)
		fmt.Println("Server stop signal sent.")
		return nil
	},
}

func init() {
	serverStartCmd.Flags().StringP("port", "p", "8080", "Port to listen on")
	serverStartCmd.Flags().String("db", "./data/oma.db", "Database path")
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
}

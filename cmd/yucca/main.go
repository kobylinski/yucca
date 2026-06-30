package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kobylinski/yucca/internal/daemon"
	yuccaExec "github.com/kobylinski/yucca/internal/exec"
	"github.com/kobylinski/yucca/internal/hook"
	yuccaInit "github.com/kobylinski/yucca/internal/init"
	"github.com/kobylinski/yucca/internal/mcp"
	"github.com/kobylinski/yucca/internal/service"
	"github.com/kobylinski/yucca/internal/store"
	tuiPkg "github.com/kobylinski/yucca/internal/tui"
	"github.com/spf13/cobra"
)

var version = "dev"

// isAgent returns true if running inside an AI agent context.
func isAgent(cmd *cobra.Command) bool {
	if f := cmd.Flag("agent"); f != nil && f.Value.String() == "true" {
		return true
	}
	for _, env := range []string{"CLAUDECODE", "CURSOR_TRACE_ID", "GITHUB_COPILOT_TOKEN", "AIDER_MODEL", "OPENCODE"} {
		if os.Getenv(env) != "" {
			return true
		}
	}
	return false
}

// agentBlock prints a markdown message and exits cleanly when a command
// is not available in agent mode. Returns true if the command was blocked.
func agentBlock(cmd *cobra.Command, message string) bool {
	if !isAgent(cmd) {
		return false
	}
	fmt.Println(message)
	return true
}

func main() {
	root := &cobra.Command{
		Use:   "yucca",
		Short: "Local secret management for AI coding assistants",
	}

	root.PersistentFlags().Bool("agent", false, "Enable agent mode (auto-detected from environment)")

	root.AddCommand(
		newDaemonCmd(),
		newMCPCmd(),
		newExecCmd(),
		newHookCmd(),
		newInitCmd(),
		newFileCmd(),
		newSecretCmd(),
		newProjectCmd(),
		newTUICmd(),
		&cobra.Command{
			Use:   "version",
			Short: "Print version",
			Run: func(cmd *cobra.Command, args []string) {
				fmt.Println(version)
			},
		},
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Yucca daemon management",
	}

	var port int
	var idleTimeout time.Duration
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Yucca daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := daemon.DefaultConfig()
			if port > 0 {
				cfg.Port = port
			}
			cfg.IdleTimeout = idleTimeout
			d, err := daemon.New(cfg)
			if err != nil {
				return fmt.Errorf("init daemon: %w", err)
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return d.Start(ctx)
		},
	}
	startCmd.Flags().IntVar(&port, "port", 9777, "HTTP port to listen on")
	startCmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 5*time.Minute, "Shutdown after no active sessions (0 to disable)")

	var stopAddr string
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := daemon.DiscoverAddr(stopAddr)
			resp, err := http.Post(addr+"/api/shutdown", "application/json", nil)
			if err != nil {
				fmt.Println("Daemon is not running.")
				return nil
			}
			resp.Body.Close()
			fmt.Println("Daemon shutting down.")
			return nil
		},
	}
	stopCmd.Flags().StringVar(&stopAddr, "addr", "", "Daemon address (auto-discovered if omitted)")

	var statusAddr string
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Check if the daemon is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := daemon.DiscoverAddr(statusAddr)
			resp, err := http.Get(addr + "/api/health")
			if err != nil {
				fmt.Println("Daemon is not running.")
				return nil
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if info := daemon.ReadLockfile(); info != nil {
					fmt.Printf("Daemon is running at %s (pid %d)\n", addr, info.PID)
				} else {
					fmt.Printf("Daemon is running at %s\n", addr)
				}
			} else {
				fmt.Println("Daemon returned unexpected status.")
			}
			return nil
		},
	}
	statusCmd.Flags().StringVar(&statusAddr, "addr", "", "Daemon address (auto-discovered if omitted)")

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the daemon as an OS-managed service (auto-start at login, auto-restart)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !service.Available() {
				return fmt.Errorf("no supported service manager found on this platform")
			}
			bin, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve binary path: %w", err)
			}
			changed, err := service.WriteDescriptor(bin)
			if err != nil {
				return fmt.Errorf("write service descriptor: %w", err)
			}
			if err := service.Start(changed); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			fmt.Printf("Yucca daemon installed as a managed service (%s).\n", bin)
			return nil
		},
	}

	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the OS-managed daemon service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := service.Remove(); err != nil {
				return fmt.Errorf("remove service: %w", err)
			}
			fmt.Println("Yucca daemon service removed.")
			return nil
		},
	}

	cmd.AddCommand(startCmd, stopCmd, statusCmd, installCmd, uninstallCmd)

	// Keep backward compat: `yucca daemon --port 9777` still works
	cmd.RunE = startCmd.RunE
	cmd.Flags().IntVar(&port, "port", 9777, "HTTP port to listen on")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 5*time.Minute, "Shutdown after no active sessions (0 to disable)")

	return cmd
}

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server commands",
	}

	var daemonAddr string
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server (stdio)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			home, _ := os.UserHomeDir()
			s, err := store.New(filepath.Join(home, ".yucca"))
			if err != nil {
				return fmt.Errorf("init store: %w", err)
			}
			addr := daemon.DiscoverAddr(daemonAddr)
			srv := mcp.New(addr, projectPath, s)
			return srv.Run()
		},
	}
	serve.Flags().StringVar(&daemonAddr, "daemon", "", "Daemon HTTP address (auto-discovered if omitted)")
	cmd.AddCommand(serve)
	return cmd
}

func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "exec -- [command]",
		Short:              "Execute a command with secrets injected",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if agentBlock(cmd, "# Command Not Available\n\n`yucca exec` is not available in agent mode.\n\n## Use Instead\n\nThe **yucca_exec** MCP tool provides the same functionality with approval flow and output masking.") {
				return nil
			}
			// Strip leading "--" if present
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			if len(args) == 0 {
				return fmt.Errorf("no command specified. Usage: yucca exec -- <command>")
			}

			projectPath := os.Getenv("YUCCA_PROJECT")
			if projectPath == "" {
				var err error
				projectPath, err = os.Getwd()
				if err != nil {
					return err
				}
			}

			home, _ := os.UserHomeDir()
			s, err := store.New(filepath.Join(home, ".yucca"))
			if err != nil {
				return err
			}

			exitCode, err := yuccaExec.Run(s, projectPath, args)
			if err != nil {
				return err
			}
			os.Exit(exitCode)
			return nil
		},
	}
	return cmd
}

func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Claude Code hook handlers",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "session-start",
			Short: "Handle SessionStart hook",
			RunE: func(cmd *cobra.Command, args []string) error {
				input, err := hook.ParseHookInput(os.Stdin)
				if err != nil {
					return err
				}
				daemonAddr := daemon.DiscoverAddr("")
				return hook.HandleSessionStart(input, daemonAddr)
			},
		},
		&cobra.Command{
			Use:   "session-end",
			Short: "Handle SessionEnd hook",
			RunE: func(cmd *cobra.Command, args []string) error {
				input, err := hook.ParseHookInput(os.Stdin)
				if err != nil {
					return err
				}
				return hook.HandleSessionEnd(input)
			},
		},
		&cobra.Command{
			Use:   "pre-tool-use",
			Short: "Handle PreToolUse hook",
			RunE: func(cmd *cobra.Command, args []string) error {
				input, err := hook.ParseHookInput(os.Stdin)
				if err != nil {
					return err
				}
				result := hook.HandlePreToolUse(input, hook.LoadProtectedPatterns(input.Cwd))
				if result != "" {
					fmt.Print(result)
				}
				return nil
			},
		},
	)
	return cmd
}

func newTUICmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Approval console for headless/SSH sessions (approve requests, provide values)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if agentBlock(cmd, "# Command Not Available\n\n`yucca tui` is an interactive terminal UI for human operators.\n\n## Use Instead\n\n- **yucca_secret_index** — list and search secrets\n- **yucca_secret_store** — store a secret value\n- **yucca_secret_request** — request a secret from the user\n- **yucca_secret_context** — read/write secret notes") {
				return nil
			}
			return tuiPkg.Run(daemon.DiscoverAddr(addr))
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "", "Daemon address (auto-discovered if omitted)")
	return cmd
}

func newInitCmd() *cobra.Command {
	var files []string
	var name string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Yucca for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := os.Getwd()
			if err != nil {
				return err
			}
			return yuccaInit.RunInit(projectPath, isAgent(cmd), files, name)
		},
	}
	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "Files to protect (bare=discover, \"file,key1,key2\"=decide)")
	cmd.Flags().StringVar(&name, "name", "", "Project name (default: directory name)")
	return cmd
}

func newFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "Manage protected files",
	}

	addCmd := &cobra.Command{
		Use:   "add [file...]",
		Short: "Add files to the protected list",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := os.Getwd()
			if err != nil {
				return err
			}
			for _, file := range args {
				if err := yuccaInit.AddProtectedFile(projectPath, file); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.AddCommand(addCmd)
	return cmd
}

func newSecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage secrets",
		// Silence cobra's own error/usage output so only the agentBlock message
		// shows; returning a non-nil error is what actually stops the subcommand
		// (returning nil from PersistentPreRunE does NOT — cobra still runs it).
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if agentBlock(cmd, "# Command Not Available\n\n`yucca secret` CLI commands are not available in agent mode.\n\n## Use Instead\n\n- **yucca_secret_index** — list and search secrets\n- **yucca_secret_store** — store a secret value\n- **yucca_secret_request** — request a secret from the user\n- **yucca_secret_context** — read/write secret notes") {
				return fmt.Errorf("yucca secret CLI is not available in agent mode")
			}
			return nil
		},
	}

	var fromProject, toProject string
	copyCmd := &cobra.Command{
		Use:   "copy [alias]",
		Short: "Copy a secret from one project to another",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := args[0]
			if fromProject == "" || toProject == "" {
				return fmt.Errorf("both --from and --to are required")
			}
			home, _ := os.UserHomeDir()
			s, err := store.New(filepath.Join(home, ".yucca"))
			if err != nil {
				return err
			}
			if err := s.CopyCredential(fromProject, toProject, alias); err != nil {
				return err
			}
			fmt.Printf("Copied %s from %s to %s\n", alias, fromProject, toProject)
			return nil
		},
	}
	copyCmd.Flags().StringVar(&fromProject, "from", "", "Source project path")
	copyCmd.Flags().StringVar(&toProject, "to", "", "Target project path")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects and their secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, _ := os.UserHomeDir()
			s, err := store.New(filepath.Join(home, ".yucca"))
			if err != nil {
				return err
			}
			projects, err := s.ListProjects()
			if err != nil {
				return err
			}
			if len(projects) == 0 {
				fmt.Println("No projects found.")
				return nil
			}
			for _, p := range projects {
				fmt.Printf("\n%s (%s)\n", p.Name, p.Path)
				creds, err := s.ListCredentials(p.Path)
				if err != nil {
					continue
				}
				if len(creds) == 0 {
					fmt.Println("  (no secrets)")
					continue
				}
				for alias, meta := range creds {
					fmt.Printf("  %-30s  policy: %s\n", alias, meta.Policy)
				}
			}
			fmt.Println()
			return nil
		},
	}

	cmd.AddCommand(copyCmd, listCmd)
	return cmd
}

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, _ := os.UserHomeDir()
			s, err := store.New(filepath.Join(home, ".yucca"))
			if err != nil {
				return err
			}
			projects, err := s.ListProjects()
			if err != nil {
				return err
			}
			if len(projects) == 0 {
				fmt.Println("No projects found.")
				return nil
			}
			if isAgent(cmd) {
				fmt.Printf("# Yucca Projects (%d)\n\n", len(projects))
				for _, p := range projects {
					creds, _ := s.ListCredentials(p.Path)
					fmt.Printf("- **%s** — `%s` (%d secrets)\n", p.Name, p.Path, len(creds))
				}
				fmt.Println("\nUse `yucca project show <path>` for details.")
			} else {
				for _, p := range projects {
					creds, _ := s.ListCredentials(p.Path)
					fmt.Printf("%-30s  %s  (%d secrets)\n", p.Name, p.Path, len(creds))
				}
			}
			return nil
		},
	}

	showCmd := &cobra.Command{
		Use:   "show [path]",
		Short: "Show project configuration and secrets",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := resolveProjectPath(args)
			if err != nil {
				return err
			}
			home, _ := os.UserHomeDir()
			s, err := store.New(filepath.Join(home, ".yucca"))
			if err != nil {
				return err
			}
			projects, err := s.ListProjects()
			if err != nil {
				return err
			}
			var found bool
			for _, p := range projects {
				if p.Path == projectPath {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("project not found: %s", projectPath)
			}
			creds, err := s.ListCredentials(projectPath)
			if err != nil {
				return err
			}
			if isAgent(cmd) {
				fmt.Printf("# %s\n\n", filepath.Base(projectPath))
				fmt.Printf("- **Path:** `%s`\n", projectPath)
				fmt.Printf("- **Secrets:** %d\n\n", len(creds))
				if len(creds) > 0 {
					fmt.Println("| Alias | Source | Policy | Context |")
					fmt.Println("|-------|--------|--------|---------|")
					for alias, meta := range creds {
						source := string(meta.Source.Type)
						if meta.Source.Type == "file" {
							source = fmt.Sprintf("file (`%s:%s`)", meta.Source.FilePath, meta.Source.FileKey)
						}
						ctx := meta.Context
						if ctx == "" {
							ctx = "—"
						}
						fmt.Printf("| `%s` | %s | %s | %s |\n", alias, source, meta.Policy, ctx)
					}
				}
			} else {
				fmt.Printf("Project: %s\n", filepath.Base(projectPath))
				fmt.Printf("Path:    %s\n", projectPath)
				fmt.Printf("Secrets: %d\n\n", len(creds))
				for alias, meta := range creds {
					fmt.Printf("  %-30s  source: %-6s  policy: %s\n", alias, meta.Source.Type, meta.Policy)
					if meta.Source.Type == "file" {
						fmt.Printf("  %-30s  file: %s  key: %s\n", "", meta.Source.FilePath, meta.Source.FileKey)
					}
					if meta.Context != "" {
						fmt.Printf("  %-30s  context: %s\n", "", meta.Context)
					}
				}
			}
			return nil
		},
	}

	syncCmd := &cobra.Command{
		Use:   "sync [path]",
		Short: "Re-read secrets from config files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := resolveProjectPath(args)
			if err != nil {
				return err
			}
			addr := daemon.DiscoverAddr("")
			slug := store.ProjectSlugFromPath(projectPath)

			secrets := loadSecretsFromYAML(projectPath)
			if len(secrets) == 0 {
				fmt.Println("No secrets defined in .yucca.yaml")
				return nil
			}

			payload, _ := json.Marshal(map[string]any{
				"project_path": projectPath,
				"project_name": filepath.Base(projectPath),
				"secrets":      secrets,
			})
			resp, err := http.Post(
				fmt.Sprintf("%s/api/projects/%s/sync", addr, slug),
				"application/json",
				bytes.NewReader(payload),
			)
			if err != nil {
				return fmt.Errorf("daemon not running: %w", err)
			}
			defer resp.Body.Close()

			var result map[string]any
			json.NewDecoder(resp.Body).Decode(&result)
			synced, _ := result["synced"].(float64)
			fmt.Printf("Synced %d secrets from .yucca.yaml\n", int(synced))
			return nil
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete [path]",
		Short: "Delete a project and all its secrets",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if agentBlock(cmd, "# Command Not Available\n\n`yucca project delete` is not available in agent mode — it permanently removes a project and all its secrets from the keychain.\n\n## Suggest to User\n\nAsk the human operator to run:\n```\nyucca project delete\n```") {
				return nil
			}
			projectPath, err := resolveProjectPath(args)
			if err != nil {
				return err
			}
			home, _ := os.UserHomeDir()
			s, err := store.New(filepath.Join(home, ".yucca"))
			if err != nil {
				return err
			}

			creds, _ := s.ListCredentials(projectPath)
			fmt.Printf("This will delete project %q and %d secrets from keychain.\n", filepath.Base(projectPath), len(creds))
			fmt.Print("Continue? [y/N] ")
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}

			if err := s.DeleteProject(projectPath); err != nil {
				return err
			}
			fmt.Println("Project deleted.")
			return nil
		},
	}

	cmd.AddCommand(listCmd, showCmd, syncCmd, deleteCmd)
	return cmd
}

func resolveProjectPath(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	return os.Getwd()
}

// loadSecretsFromYAML reads .yucca.yaml and returns secret definitions.
func loadSecretsFromYAML(projectPath string) []map[string]string {
	data, err := os.ReadFile(filepath.Join(projectPath, ".yucca.yaml"))
	if err != nil {
		return nil
	}
	var secrets []map[string]string
	lines := strings.Split(string(data), "\n")
	inSecrets := false
	var currentFile string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "secrets:" {
			inSecrets = true
			continue
		}
		if !inSecrets {
			continue
		}
		if strings.HasPrefix(trimmed, "- file: ") {
			currentFile = strings.TrimPrefix(trimmed, "- file: ")
		} else if strings.HasPrefix(trimmed, "key: ") {
			key := strings.TrimPrefix(trimmed, "key: ")
			if currentFile != "" && key != "" {
				secrets = append(secrets, map[string]string{"file": currentFile, "key": key})
			}
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") &&
			!strings.HasPrefix(trimmed, "file:") && !strings.HasPrefix(trimmed, "key:") {
			break
		}
	}
	return secrets
}

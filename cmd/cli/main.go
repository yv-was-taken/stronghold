package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"stronghold/internal/cli"
)

var (
	version   = "dev"
	commit    = "unknown"
	date      = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "stronghold",
		Short: "Stronghold - AI security proxy for LLM agents",
		Long: `Stronghold is a system-wide HTTP/HTTPS proxy that scans all outbound
traffic for prompt injection attacks and credential leaks before they reach
your AI agents.

Designed for isolated machines running AI agents. Not recommended for daily-use
workstations as it intercepts ALL system traffic.

Quick Start:
  stronghold init       # Initialize Stronghold proxy and account
  stronghold status     # Check proxy status
  stronghold enable     # Enable protection
  stronghold disable    # Disable protection

For more information, visit https://stronghold.security`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	// Init command
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Stronghold proxy and account",
		Long: `Initialize Stronghold proxy with interactive setup.

This command will:
  1. Check system compatibility
  2. Create or login to your Stronghold account
  3. Set up your wallet (new or imported)
  4. Configure proxy settings
  5. Install system service
  6. Start the proxy

WARNING: This sets up a system-wide proxy that will route ALL traffic
through Stronghold's scanning service. Intended for isolated machines only.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nonInteractive, _ := cmd.Flags().GetBool("yes")
			privateKey, _ := cmd.Flags().GetString("private-key")
			accountNumber, _ := cmd.Flags().GetString("account-number")
			if !nonInteractive && (privateKey != "" || accountNumber != "") {
				fmt.Println(cli.WarningStyle.Render("Warning:"), "--private-key and --account-number require --yes flag")
				fmt.Println("Running interactive mode instead. Use --yes for non-interactive.")
			}
			if nonInteractive {
				return cli.RunInstallNonInteractive(privateKey, accountNumber)
			}
			return cli.RunInstall()
		},
	}
	initCmd.Flags().BoolP("yes", "y", false, "Non-interactive mode (skips prompts, uses defaults)")
	initCmd.Flags().String("private-key", "", "Import wallet from private key (hex) - requires --yes")
	initCmd.Flags().String("account-number", "", "Login to existing account - requires --yes")

	// Enable command
	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable Stronghold protection",
		Long: `Start the Stronghold proxy and enable system-wide protection.

This will:
  1. Start the Stronghold proxy daemon
  2. Configure transparent proxy using iptables/nftables (Linux) or pf (macOS)
  3. Intercept all HTTP/HTTPS traffic at the network level

The transparent proxy cannot be bypassed by applications and requires
root/admin privileges to configure firewall rules.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.Enable()
		},
	}

	// Disable command
	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable Stronghold protection",
		Long: `Stop the Stronghold proxy and restore direct internet access.

This will:
  1. Remove system proxy configuration
  2. Stop the Stronghold proxy daemon
  3. Restore direct internet access`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.Disable()
		},
	}

	// Status command
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show Stronghold status",
		Long:  `Display the current status of the Stronghold proxy, including protection status, usage statistics, and configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.Status()
		},
	}

	// Uninstall command
	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Stronghold from the system",
		Long: `Completely remove Stronghold proxy and configuration.

This will:
  1. Stop the proxy
  2. Remove system proxy configuration
  3. Remove system service
  4. Remove binaries
  5. Optionally remove configuration

Your wallet balance will be preserved unless you explicitly delete your account.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			preserve, _ := cmd.Flags().GetBool("preserve-config")
			return cli.Uninstall(preserve)
		},
	}
	uninstallCmd.Flags().BoolP("preserve-config", "p", true, "Preserve configuration files")

	// Logs command
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "View proxy logs",
		Long:  `Display the Stronghold proxy logs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			follow, _ := cmd.Flags().GetBool("follow")
			lines, _ := cmd.Flags().GetInt("lines")
			return cli.Logs(follow, lines)
		},
	}
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output (like tail -f)")
	logsCmd.Flags().IntP("lines", "n", 100, "Number of lines to show")

	// Config command
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Stronghold configuration",
		Long:  `View and modify Stronghold configuration settings.`,
	}

	configGetCmd := &cobra.Command{
		Use:   "get",
		Short: "Get configuration value",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := cli.LoadConfig()
			if err != nil {
				return err
			}
			fmt.Printf("Config file: %s\n", cli.ConfigPath())
			fmt.Printf("Proxy port: %d\n", config.Proxy.Port)
			fmt.Printf("API endpoint: %s\n", config.API.Endpoint)
			fmt.Printf("Logged in: %v\n", config.Auth.LoggedIn)
			if config.Auth.LoggedIn {
				fmt.Printf("User: %s\n", config.Auth.Email)
				if config.Wallet.Address != "" {
					fmt.Printf("Account: %s\n", config.Wallet.Address)
				}
			}
			return nil
		},
	}

	configCmd.AddCommand(configGetCmd)

	// Account command
	accountCmd := &cobra.Command{
		Use:   "account",
		Short: "Manage your Stronghold account",
		Long:  `View balance, deposit funds, and manage your account.`,
	}

	accountBalanceCmd := &cobra.Command{
		Use:   "balance",
		Short: "Check your account balance",
		Long:  `Display your current balance and account status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.AccountBalance()
		},
	}

	accountDepositCmd := &cobra.Command{
		Use:   "deposit",
		Short: "Add funds to your account",
		Long: `Show deposit options to add funds to your account.

You can deposit via:
  - Dashboard: Use Stripe, Coinbase Pay, or Moonpay (recommended)
  - Direct: Send USDC directly to your account address`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.AccountDeposit()
		},
	}

	accountCmd.AddCommand(accountBalanceCmd, accountDepositCmd)

	// Wallet command
	walletCmd := &cobra.Command{
		Use:   "wallet",
		Short: "Manage your Stronghold wallet",
	}

	walletExportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export wallet private key to a file",
		Long: `Export your wallet private key to a file for backup.

By default, exports to ~/.stronghold/wallet-backup
Use --output to specify a different location.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			return cli.ExportWallet(output)
		},
	}
	walletExportCmd.Flags().StringP("output", "o", "", "Output file path (default: ~/.stronghold/wallet-backup)")

	walletCmd.AddCommand(walletExportCmd)

	// Doctor command
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check system prerequisites",
		Long: `Run diagnostic checks to verify your system is ready for Stronghold.

This command checks:
  - Operating system compatibility (Linux/macOS)
  - Root/admin privileges
  - Firewall tools (iptables/nftables on Linux, pf on macOS)
  - Kernel modules (Linux)
  - Available ports
  - Configuration permissions
  - Binary installations

Run this before 'stronghold init' to catch issues early.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.Doctor()
		},
	}

	// Wallet command group
	walletCmd := &cobra.Command{
		Use:   "wallet",
		Short: "Manage wallet",
		Long:  `View and manage your Stronghold wallet.`,
	}

	walletReplaceCmd := &cobra.Command{
		Use:   "replace",
		Short: "Replace wallet with a new private key",
		Long: `Replace your current wallet with a new one.

Reads the private key from (in order of precedence):
  1. stdin (if piped)
  2. STRONGHOLD_PRIVATE_KEY environment variable
  3. --file flag
  4. Interactive prompt (if terminal)

Example:
  echo $KEY | stronghold wallet replace --yes
  STRONGHOLD_PRIVATE_KEY=xxx stronghold wallet replace --yes
  stronghold wallet replace --file /path/to/key.txt
  stronghold wallet replace  # interactive prompt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileFlag, _ := cmd.Flags().GetString("file")
			yesFlag, _ := cmd.Flags().GetBool("yes")
			return cli.WalletReplace(fileFlag, yesFlag)
		},
	}
	walletReplaceCmd.Flags().StringP("file", "f", "", "Read private key from file")
	walletReplaceCmd.Flags().BoolP("yes", "y", false, "Skip warnings and confirmations")

	walletCmd.AddCommand(walletReplaceCmd)

	// Add all commands
	rootCmd.AddCommand(
		initCmd,
		enableCmd,
		disableCmd,
		statusCmd,
		uninstallCmd,
		logsCmd,
		configCmd,
		accountCmd,
		walletCmd,
		doctorCmd,
	)

	// Execute
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

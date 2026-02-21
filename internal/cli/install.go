package cli

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InstallState represents the current state of the installation
type InstallState int

const (
	StateWarning InstallState = iota
	StateChecking
	StateAccount
	StatePayment
	StateConfig
	StateInstalling
	StateComplete
	StateError
)

// InstallModel is the Bubble Tea model for the install command
type InstallModel struct {
	state  InstallState
	config *CLIConfig
	width  int
	height int

	// Warning step
	confirmWarning bool

	// Account step
	accountChoice          int // 0 = create, 1 = create with existing wallet, 2 = existing account, 3 = skip
	accountNumber          string
	walletAddress          string
	authToken              string
	loggedIn               bool
	importKeyInput         textinput.Model
	importSolanaKeyInput   textinput.Model
	loginInput             textinput.Model
	awaitingKeyInput       bool
	awaitingSolanaKeyInput bool
	awaitingLoginInput     bool

	// Payment step
	paymentMethod int // 0 = stripe, 1 = wallet

	// Config step
	portInput  textinput.Model
	apiInput   textinput.Model
	configPort int
	configAPI  string

	// Progress
	progress    []string
	currentStep int
	errorMsg    string
}

// Styles - exported styles have capitalized names
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00D4AA")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D4AA"))

	// WarningStyle is exported for use in main.go
	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500"))
	warningStyle = WarningStyle // internal alias

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4444"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D4AA")).
			Bold(true)

	unselectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))
)

// NewInstallModel creates a new install model
func NewInstallModel() *InstallModel {
	portInput := textinput.New()
	portInput.Placeholder = "8402"
	portInput.CharLimit = PortInputCharLimit
	portInput.Width = 10

	apiInput := textinput.New()
	apiInput.Placeholder = DefaultAPIEndpoint
	apiInput.CharLimit = APIInputCharLimit
	apiInput.Width = APIInputWidth

	importKeyInput := textinput.New()
	importKeyInput.Placeholder = "Enter EVM private key (hex)"
	importKeyInput.CharLimit = PrivateKeyInputCharLimit
	importKeyInput.Width = PrivateKeyInputWidth
	importKeyInput.EchoMode = textinput.EchoPassword
	importKeyInput.EchoCharacter = '*'

	importSolanaKeyInput := textinput.New()
	importSolanaKeyInput.Placeholder = "Enter Solana private key (base58) or press Enter to skip"
	importSolanaKeyInput.CharLimit = SolanaPrivateKeyInputCharLimit
	importSolanaKeyInput.Width = SolanaPrivateKeyInputWidth
	importSolanaKeyInput.EchoMode = textinput.EchoPassword
	importSolanaKeyInput.EchoCharacter = '*'

	loginInput := textinput.New()
	loginInput.Placeholder = "XXXX-XXXX-XXXX-XXXX"
	loginInput.CharLimit = AccountNumberInputCharLimit
	loginInput.Width = AccountNumberInputWidth

	return &InstallModel{
		state:                StateWarning,
		config:               DefaultConfig(),
		portInput:            portInput,
		apiInput:             apiInput,
		importKeyInput:       importKeyInput,
		importSolanaKeyInput: importSolanaKeyInput,
		loginInput:           loginInput,
		configPort:           DefaultProxyPort,
		configAPI:            DefaultAPIEndpoint,
		progress:             []string{},
	}
}

// Init initializes the model
func (m *InstallModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *InstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			// Handle escape in input modes
			if m.state == StateAccount && m.awaitingKeyInput {
				m.awaitingKeyInput = false
				m.importKeyInput.SetValue("") // Clear sensitive data
				m.importKeyInput.Blur()
				return m, nil
			}
			if m.state == StateAccount && m.awaitingSolanaKeyInput {
				m.awaitingSolanaKeyInput = false
				m.importSolanaKeyInput.SetValue("")
				m.importSolanaKeyInput.Blur()
				return m, nil
			}
			if m.state == StateAccount && m.awaitingLoginInput {
				m.awaitingLoginInput = false
				m.loginInput.SetValue("") // Clear for cleaner re-entry
				m.loginInput.Blur()
				return m, nil
			}
			return m, tea.Quit

		case "enter":
			return m.handleEnter()

		case "y", "Y":
			if m.state == StateWarning {
				m.confirmWarning = true
				m.state = StateChecking
				return m, m.runChecks()
			}

		case "n", "N":
			if m.state == StateWarning {
				return m, tea.Quit
			}

		case "up", "k":
			m.handleUp()

		case "down", "j":
			m.handleDown()

		case "tab":
			m.handleTab()
		}
	}

	// Update text inputs
	var cmd tea.Cmd
	switch m.state {
	case StateAccount:
		if m.awaitingKeyInput {
			m.importKeyInput, cmd = m.importKeyInput.Update(msg)
		} else if m.awaitingSolanaKeyInput {
			m.importSolanaKeyInput, cmd = m.importSolanaKeyInput.Update(msg)
		} else if m.awaitingLoginInput {
			m.loginInput, cmd = m.loginInput.Update(msg)
		}
	case StateConfig:
		if m.portInput.Focused() {
			m.portInput, cmd = m.portInput.Update(msg)
		} else {
			m.apiInput, cmd = m.apiInput.Update(msg)
		}
	}

	return m, cmd
}

// View renders the UI
func (m *InstallModel) View() string {
	switch m.state {
	case StateWarning:
		return m.viewWarning()
	case StateChecking:
		return m.viewChecking()
	case StateAccount:
		return m.viewAccount()
	case StatePayment:
		return m.viewPayment()
	case StateConfig:
		return m.viewConfig()
	case StateInstalling:
		return m.viewInstalling()
	case StateComplete:
		return m.viewComplete()
	case StateError:
		return m.viewError()
	default:
		return ""
	}
}

// handleEnter processes the enter key
func (m *InstallModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateChecking:
		m.state = StateAccount
		return m, nil

	case StateAccount:
		// Handle EVM key input submission
		if m.awaitingKeyInput {
			privateKey := m.importKeyInput.Value()
			if privateKey == "" {
				return m, nil
			}
			// Import the provided EVM private key
			userID := generateUserID()
			m.config.Auth.UserID = userID
			address, err := ImportWallet(userID, DefaultBlockchain, privateKey)
			if err != nil {
				m.progress = append(m.progress, errorStyle.Render(fmt.Sprintf("✗ Invalid EVM private key: %v", err)))
				m.importKeyInput.SetValue("") // Clear invalid key
				m.awaitingKeyInput = false
				return m, nil
			}
			m.config.Wallet.Address = address
			m.config.Wallet.Network = DefaultBlockchain
			m.walletAddress = address
			m.progress = append(m.progress, successStyle.Render(fmt.Sprintf("✓ EVM wallet imported: %s", address)))

			// Now prompt for Solana key
			m.awaitingKeyInput = false
			m.awaitingSolanaKeyInput = true
			m.importSolanaKeyInput.Focus()
			return m, nil
		}

		// Handle Solana key input submission
		if m.awaitingSolanaKeyInput {
			solanaKey := m.importSolanaKeyInput.Value()
			userID := m.config.Auth.UserID

			if solanaKey != "" {
				// Import the provided Solana private key
				solanaAddr, err := ImportSolanaWallet(userID, DefaultSolanaNetwork, solanaKey)
				if err != nil {
					m.progress = append(m.progress, errorStyle.Render(fmt.Sprintf("✗ Invalid Solana private key: %v", err)))
					m.importSolanaKeyInput.SetValue("")
					// Stay in Solana key input state so the user can retry
					return m, nil
				}
				m.config.Wallet.SolanaAddress = solanaAddr
				m.config.Wallet.SolanaNetwork = DefaultSolanaNetwork
				m.progress = append(m.progress, successStyle.Render(fmt.Sprintf("✓ Solana wallet imported: %s", solanaAddr)))
			} else {
				// No Solana key provided, create a new one
				solanaAddr, solanaErr := SetupSolanaWallet(userID, DefaultSolanaNetwork)
				if solanaErr != nil {
					m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ Solana wallet setup failed: %v", solanaErr)))
				} else {
					m.config.Wallet.SolanaAddress = solanaAddr
					m.config.Wallet.SolanaNetwork = DefaultSolanaNetwork
					m.progress = append(m.progress, successStyle.Render(fmt.Sprintf("✓ Solana wallet created: %s", solanaAddr)))
				}
			}

			// Create account via API with the EVM wallet
			address := m.config.Wallet.Address
			apiClient := NewAPIClient(m.config.API.Endpoint, m.config.Auth.DeviceToken)
			resp, err := apiClient.CreateAccount(&CreateAccountRequest{WalletAddress: &address})
			if err != nil {
				m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ API unavailable: %v", err)))
				m.accountNumber = generateSimulatedAccountNumber()
			} else {
				m.accountNumber = resp.AccountNumber
				m.progress = append(m.progress, successStyle.Render("✓ Account created via API"))

				// Register wallet addresses with server (best-effort)
				if err := apiClient.RegisterWalletAddresses(m.config.Wallet.Address, m.config.Wallet.SolanaAddress); err == nil {
					m.progress = append(m.progress, successStyle.Render("✓ Wallet addresses registered with server"))
				}
			}
			m.config.Auth.AccountNumber = m.accountNumber
			m.config.Auth.LoggedIn = true
			m.loggedIn = true
			m.awaitingSolanaKeyInput = false
			m.state = StatePayment
			return m, nil
		}

		// Handle login input submission
		if m.awaitingLoginInput {
			accountNum := m.loginInput.Value()
			if accountNum == "" {
				return m, nil
			}
			apiClient := NewAPIClient(m.config.API.Endpoint, m.config.Auth.DeviceToken)
			loginResp, err := apiClient.Login(accountNum)
			if err != nil {
				m.progress = append(m.progress, errorStyle.Render(fmt.Sprintf("✗ Login failed: %v", err)))
				m.loginInput.SetValue("") // Clear invalid account number
				m.awaitingLoginInput = false
				return m, nil
			}
			m.accountNumber = loginResp.AccountNumber
			m.config.Auth.AccountNumber = loginResp.AccountNumber
			m.config.Auth.LoggedIn = true
			m.loggedIn = true
			m.progress = append(m.progress, successStyle.Render(fmt.Sprintf("✓ Logged in as %s", loginResp.AccountNumber)))

			if err := ensureTrustedDevice(apiClient, m.config, loginResp.TOTPRequired); err != nil {
				m.progress = append(m.progress, errorStyle.Render(fmt.Sprintf("✗ TOTP verification failed: %v", err)))
				m.awaitingLoginInput = false
				return m, nil
			}

			// Reset local wallet linkage and repopulate from server response.
			m.config.Wallet.Address = ""
			m.walletAddress = ""
			m.config.Wallet.SolanaAddress = ""

			evmAddr := loginResp.EVMWalletAddress
			if evmAddr != nil {
				if loginResp.EscrowEnabled {
					privateKey, err := apiClient.GetWalletKey()
					if err != nil {
						m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ Wallet key fetch failed: %v", err)))
						m.config.Wallet.Address = *evmAddr
						m.config.Wallet.Network = DefaultBlockchain
						m.walletAddress = *evmAddr
					} else {
						userID := generateUserID()
						m.config.Auth.UserID = userID
						address, err := ImportWallet(userID, DefaultBlockchain, privateKey)
						// Zero the private key after use
						ZeroString(&privateKey)
						if err != nil {
							m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ Wallet import failed: %v", err)))
						} else {
							m.config.Wallet.Address = address
							m.config.Wallet.Network = DefaultBlockchain
							m.walletAddress = address
							m.progress = append(m.progress, successStyle.Render("✓ Wallet synced"))
						}
					}
				} else {
					m.config.Wallet.Address = *evmAddr
					m.config.Wallet.Network = DefaultBlockchain
					m.walletAddress = *evmAddr
					m.progress = append(m.progress, warningStyle.Render("⚠ Wallet not stored on server. Import locally to enable payments."))
				}
			}
			if loginResp.SolanaWalletAddress != nil {
				m.config.Wallet.SolanaAddress = *loginResp.SolanaWalletAddress
				if m.config.Wallet.SolanaNetwork == "" {
					m.config.Wallet.SolanaNetwork = DefaultSolanaNetwork
				}
				m.progress = append(m.progress, successStyle.Render(fmt.Sprintf("✓ Solana wallet linked: %s", *loginResp.SolanaWalletAddress)))
			}

			// Register wallet addresses with server (best-effort)
			if err := apiClient.RegisterWalletAddresses(m.config.Wallet.Address, m.config.Wallet.SolanaAddress); err != nil {
				m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ Wallet registration: %v", err)))
			} else if m.config.Wallet.Address != "" || m.config.Wallet.SolanaAddress != "" {
				m.progress = append(m.progress, successStyle.Render("✓ Wallet addresses registered with server"))
			}

			m.awaitingLoginInput = false
			m.state = StatePayment
			return m, nil
		}

		if m.accountChoice == AccountChoiceCreate { // Create new account
			// Create a local wallet first, then register its address with the API.
			apiClient := NewAPIClient(m.config.API.Endpoint, m.config.Auth.DeviceToken)
			walletAddress := m.config.Wallet.Address
			if walletAddress == "" {
				userID := m.config.Auth.UserID
				if userID == "" {
					userID = generateUserID()
					m.config.Auth.UserID = userID
				}
				address, err := SetupWallet(userID, "base")
				if err != nil {
					m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ EVM wallet setup failed: %v", err)))
				} else {
					walletAddress = address
					m.config.Wallet.Address = address
					m.config.Wallet.Network = DefaultBlockchain
					m.walletAddress = address
					m.progress = append(m.progress, successStyle.Render("✓ EVM wallet created locally"))
				}

				// Also create Solana wallet
				solanaAddr, solanaErr := SetupSolanaWallet(userID, DefaultSolanaNetwork)
				if solanaErr != nil {
					m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ Solana wallet setup failed: %v", solanaErr)))
				} else {
					m.config.Wallet.SolanaAddress = solanaAddr
					m.config.Wallet.SolanaNetwork = DefaultSolanaNetwork
					m.progress = append(m.progress, successStyle.Render("✓ Solana wallet created locally"))
				}
			}

			req := &CreateAccountRequest{}
			if walletAddress != "" {
				req.WalletAddress = &walletAddress
			}

			resp, err := apiClient.CreateAccount(req)
			if err != nil {
				// API failed, fall back to local wallet creation
				m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ API unavailable: %v", err)))

				// Generate simulated account number for offline mode
				m.accountNumber = generateSimulatedAccountNumber()
				m.config.Auth.AccountNumber = m.accountNumber
				m.config.Auth.LoggedIn = true
				m.loggedIn = true
			} else {
				// API success - account created with local wallet address (if available)
				m.accountNumber = resp.AccountNumber
				m.config.Auth.AccountNumber = resp.AccountNumber
				m.config.Auth.LoggedIn = true
				m.loggedIn = true
				m.progress = append(m.progress, successStyle.Render("✓ Account created via API"))
				if walletAddress == "" {
					m.progress = append(m.progress, warningStyle.Render("⚠ Account created without a wallet. Configure a local wallet to enable payments."))
				}

				// Register wallet addresses with server (best-effort)
				if err := apiClient.RegisterWalletAddresses(m.config.Wallet.Address, m.config.Wallet.SolanaAddress); err == nil {
					m.progress = append(m.progress, successStyle.Render("✓ Wallet addresses registered with server"))
				}
			}

			m.state = StatePayment
		} else if m.accountChoice == AccountChoiceCreateWithKey { // Create new account with existing wallet
			m.awaitingKeyInput = true
			m.importKeyInput.Focus()
			return m, nil
		} else if m.accountChoice == AccountChoiceExistingAccount { // Use existing account (login)
			m.awaitingLoginInput = true
			m.loginInput.Focus()
			return m, nil
		} else { // Skip
			m.state = StatePayment
		}

	case StatePayment:
		// Payment method is now always the embedded wallet
		m.config.Payments.Method = "wallet"
		m.state = StateConfig
		m.portInput.Focus()

	case StateConfig:
		// Parse port
		if m.portInput.Value() != "" {
			fmt.Sscanf(m.portInput.Value(), "%d", &m.configPort)
		}
		m.config.Proxy.Port = m.configPort

		if m.apiInput.Value() != "" {
			m.configAPI = m.apiInput.Value()
		}
		m.config.API.Endpoint = m.configAPI

		m.state = StateInstalling
		return m, m.runInstallation()

	case StateComplete:
		return m, tea.Quit

	case StateError:
		return m, tea.Quit
	}

	return m, nil
}

// handleUp handles up arrow
func (m *InstallModel) handleUp() {
	switch m.state {
	case StateAccount:
		if !m.awaitingKeyInput && !m.awaitingSolanaKeyInput && !m.awaitingLoginInput && m.accountChoice > 0 {
			m.accountChoice--
		}
	case StatePayment:
		if m.paymentMethod > 0 {
			m.paymentMethod--
		}
	}
}

// handleDown handles down arrow
func (m *InstallModel) handleDown() {
	switch m.state {
	case StateAccount:
		if !m.awaitingKeyInput && !m.awaitingSolanaKeyInput && !m.awaitingLoginInput && m.accountChoice < MaxAccountChoices-1 {
			m.accountChoice++
		}
	case StatePayment:
		if m.paymentMethod < 1 {
			m.paymentMethod++
		}
	}
}

// handleTab handles tab key
func (m *InstallModel) handleTab() {
	switch m.state {
	case StateConfig:
		if m.portInput.Focused() {
			m.portInput.Blur()
			m.apiInput.Focus()
		} else {
			m.apiInput.Blur()
			m.portInput.Focus()
		}
	}
}

// viewWarning renders the warning screen
func (m *InstallModel) viewWarning() string {
	var b strings.Builder

	b.WriteString(warningStyle.Render("⚠️  WARNING"))
	b.WriteString("\n\n")
	b.WriteString("Stronghold sets up a system-wide proxy.\n")
	b.WriteString("This will route ALL system traffic through our scanning service.\n")
	b.WriteString("This is intended for isolated machines running AI agents.\n")
	b.WriteString(warningStyle.Render("Not recommended for daily-use workstations."))
	b.WriteString("\n\n")
	b.WriteString("Continue? [y/N]: ")

	return b.String()
}

// viewChecking renders the system check screen
func (m *InstallModel) viewChecking() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Checking system..."))
	b.WriteString("\n\n")

	for _, step := range m.progress {
		b.WriteString(step)
		b.WriteString("\n")
	}

	if m.currentStep >= 2 {
		b.WriteString("\n")
		b.WriteString(infoStyle.Render("Press Enter to continue..."))
	}

	return b.String()
}

// viewAccount renders the account setup screen
func (m *InstallModel) viewAccount() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Account Setup"))
	b.WriteString("\n\n")

	// Show EVM key input if awaiting
	if m.awaitingKeyInput {
		b.WriteString("Enter your Base (EVM) private key (hex):\n")
		b.WriteString(m.importKeyInput.View())
		b.WriteString("\n\n")
		b.WriteString(infoStyle.Render("Press Enter to continue, Esc to go back"))
		return b.String()
	}

	// Show Solana key input if awaiting
	if m.awaitingSolanaKeyInput {
		for _, step := range m.progress {
			b.WriteString(step)
			b.WriteString("\n")
		}
		b.WriteString("\nEnter your Solana private key (base58), or press Enter to generate a new one:\n")
		b.WriteString(m.importSolanaKeyInput.View())
		b.WriteString("\n\n")
		b.WriteString(infoStyle.Render("Press Enter to continue (empty = generate new), Esc to go back"))
		return b.String()
	}

	// Show account number input if awaiting
	if m.awaitingLoginInput {
		b.WriteString("Enter your account number:\n")
		b.WriteString(m.loginInput.View())
		b.WriteString("\n\n")
		b.WriteString(infoStyle.Render("Press Enter to continue, Esc to go back"))
		return b.String()
	}

	b.WriteString("Stronghold uses Mullvad-style authentication:\n")
	b.WriteString("• No email or password required\n")
	b.WriteString("• 16-digit account number (XXXX-XXXX-XXXX-XXXX)\n")
	b.WriteString("• Account number is your only credential\n\n")

	// Account choices - updated to 4 options
	choices := []string{
		"Create new account",
		"Create new account with existing wallet",
		"I have an existing account",
		"Skip for now",
	}
	for i, choice := range choices {
		if i == m.accountChoice {
			b.WriteString(selectedStyle.Render(fmt.Sprintf("▸ %s", choice)))
		} else {
			b.WriteString(unselectedStyle.Render(fmt.Sprintf("  %s", choice)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(infoStyle.Render("Press Enter to continue"))

	return b.String()
}

// viewPayment renders the payment setup screen
func (m *InstallModel) viewPayment() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Payment Setup"))
	b.WriteString("\n\n")

	if m.config.Wallet.Address != "" {
		b.WriteString("Your Stronghold account has been created:\n\n")
		if m.accountNumber != "" {
			b.WriteString("Account Number:\n")
			b.WriteString(selectedStyle.Render(fmt.Sprintf("  %s", m.accountNumber)))
			b.WriteString("\n\n")
		}
		b.WriteString("Wallet Address (for deposits):\n")
		b.WriteString(selectedStyle.Render(fmt.Sprintf("  %s", m.config.Wallet.Address)))
		b.WriteString("\n\n")
		b.WriteString("Fund your wallet to start using Stronghold:\n\n")
		b.WriteString(infoStyle.Render("  1. Visit https://getstronghold.xyz/dashboard"))
		b.WriteString("\n")
		b.WriteString(infoStyle.Render("  2. Login with your account number"))
		b.WriteString("\n")
		b.WriteString(infoStyle.Render("  3. Use Stripe on-ramp or send USDC directly"))
		b.WriteString("\n\n")
		b.WriteString("Or use: stronghold account deposit")
	} else {
		b.WriteString("No wallet configured. You can skip funding for now,")
		b.WriteString("\nbut you'll need to add funds before using Stronghold.")
	}

	b.WriteString("\n\n")
	b.WriteString(infoStyle.Render("Press Enter to continue"))

	return b.String()
}

// viewConfig renders the configuration screen
func (m *InstallModel) viewConfig() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Configuration"))
	b.WriteString("\n\n")

	b.WriteString("Proxy port [8402]:\n")
	b.WriteString(m.portInput.View())
	b.WriteString("\n\n")

	b.WriteString("Stronghold API [https://api.getstronghold.xyz]:\n")
	b.WriteString(m.apiInput.View())
	b.WriteString("\n\n")

	b.WriteString(infoStyle.Render("Use Tab to switch fields, Enter to continue"))

	return b.String()
}

// viewInstalling renders the installation progress screen
func (m *InstallModel) viewInstalling() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Installing..."))
	b.WriteString("\n\n")

	for _, step := range m.progress {
		b.WriteString(step)
		b.WriteString("\n")
	}

	return b.String()
}

// viewComplete renders the completion screen
func (m *InstallModel) viewComplete() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("✓ Installation complete!"))
	b.WriteString("\n\n")
	b.WriteString("Your system is now protected. All HTTP/HTTPS traffic is being\n")
	b.WriteString("intercepted at the network level and scanned for prompt injection\n")
	b.WriteString("attacks before reaching your agents.\n\n")
	b.WriteString("This cannot be bypassed by applications.\n\n")

	if m.config.Wallet.Address != "" || m.config.Wallet.SolanaAddress != "" {
		b.WriteString(headerStyle.Render("Your Account:"))
		b.WriteString("\n")
		if m.accountNumber != "" {
			b.WriteString(fmt.Sprintf("Account Number: %s\n", m.accountNumber))
		}
		if m.config.Wallet.Address != "" {
			b.WriteString(fmt.Sprintf("Base Wallet:    %s\n", m.config.Wallet.Address))
		}
		if m.config.Wallet.SolanaAddress != "" {
			b.WriteString(fmt.Sprintf("Solana Wallet:  %s\n", m.config.Wallet.SolanaAddress))
		}
		b.WriteString("\n")
		b.WriteString(infoStyle.Render("Fund with USDC on Base or Solana to start scanning.\n"))
		b.WriteString(infoStyle.Render("Use: stronghold account deposit\n"))
		b.WriteString(infoStyle.Render("Or visit: https://getstronghold.xyz/dashboard\n\n"))
	}

	b.WriteString(headerStyle.Render("Quick Commands:"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Status:    stronghold status\n"))
	b.WriteString(fmt.Sprintf("  Wallets:   stronghold wallet list\n"))
	b.WriteString(fmt.Sprintf("  Balance:   stronghold wallet balance\n"))
	b.WriteString(fmt.Sprintf("  Upload:    stronghold wallet replace evm  (choose upload when prompted)\n"))
	b.WriteString(fmt.Sprintf("  Disable:   stronghold disable\n"))
	b.WriteString(fmt.Sprintf("  Dashboard: https://getstronghold.xyz/dashboard\n"))
	b.WriteString(fmt.Sprintf("  Usage:     ~$0.001 per scanned request\n\n"))

	b.WriteString(infoStyle.Render("Press Enter to exit"))

	return b.String()
}

// viewError renders the error screen
func (m *InstallModel) viewError() string {
	var b strings.Builder

	b.WriteString(errorStyle.Render("✗ Installation failed"))
	b.WriteString("\n\n")
	b.WriteString(m.errorMsg)
	b.WriteString("\n\n")
	b.WriteString(infoStyle.Render("Press Enter to exit"))

	return b.String()
}

// runChecks runs system checks
func (m *InstallModel) runChecks() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(SystemCheckDelay)

		// Check platform
		if !IsSupportedPlatform() {
			m.progress = append(m.progress, errorStyle.Render("✗ "+runtime.GOOS+" is not supported (Linux/macOS only)"))
			m.state = StateError
			m.errorMsg = "Unsupported operating system"
			return nil
		}
		m.progress = append(m.progress, successStyle.Render("✓ "+runtime.GOOS+" detected"))

		time.Sleep(PostCheckDelay)

		// Check port availability
		if !m.config.IsPortAvailable() {
			// Try to find an available port
			newPort := FindAvailablePort(m.config.Proxy.Port + 1)
			if newPort == 0 {
				m.progress = append(m.progress, errorStyle.Render("✗ No available ports found"))
				m.state = StateError
				m.errorMsg = "Could not find an available port"
				return nil
			}
			oldPort := m.config.Proxy.Port
			m.config.Proxy.Port = newPort
			m.progress = append(m.progress, warningStyle.Render(fmt.Sprintf("⚠ Port %d in use, using port %d", oldPort, newPort)))
		} else {
			m.progress = append(m.progress, successStyle.Render(fmt.Sprintf("✓ Port %d available", m.config.Proxy.Port)))
		}

		m.currentStep = 2
		return nil
	}
}

// runInstallation performs the installation
func (m *InstallModel) runInstallation() tea.Cmd {
	return func() tea.Msg {
		steps := []struct {
			name string
			fn   func() error
		}{
			{"Creating stronghold user", m.createUser},
			{"Saving configuration", m.saveConfig},
			{"Installing proxy binary", m.installProxyBinary},
			{"Installing CLI binary", m.installCLIBinary},
			{"Generating CA certificate", m.generateCA},
			{"Installing CA certificate", m.installCA},
			{"Configuring system service", m.configureService},
			{"Starting proxy", m.startProxy},
			{"Enabling transparent proxy", m.enableTransparentProxy},
		}

		for _, step := range steps {
			m.progress = append(m.progress, fmt.Sprintf("  → %s...", step.name))
			if err := step.fn(); err != nil {
				m.progress = append(m.progress, errorStyle.Render(fmt.Sprintf("    ✗ %s", err.Error())))
				m.state = StateError
				m.errorMsg = err.Error()
				return nil
			}
			// Replace the "→" with "✓"
			m.progress[len(m.progress)-1] = successStyle.Render(fmt.Sprintf("    ✓ %s", step.name))
			time.Sleep(InstallStepDelay)
		}

		m.config.Installed = true
		m.config.InstallDate = time.Now().Format(time.RFC3339)
		m.config.Save()

		m.state = StateComplete
		return nil
	}
}

// createUser creates the stronghold system user
func (m *InstallModel) createUser() error {
	return CreateStrongholdUser()
}

// saveConfig saves the configuration
func (m *InstallModel) saveConfig() error {
	return m.config.Save()
}

// generateCA generates the root CA certificate for MITM
func (m *InstallModel) generateCA() error {
	caDir := filepath.Join(ConfigDir(), "ca")
	if err := os.MkdirAll(caDir, 0700); err != nil {
		return fmt.Errorf("failed to create CA directory: %w", err)
	}

	certPath := filepath.Join(caDir, "ca.crt")
	keyPath := filepath.Join(caDir, "ca.key")

	// Check if CA already exists
	if _, err := os.Stat(certPath); err == nil {
		return nil // CA already exists
	}

	// Generate CA using openssl (available on both Linux and macOS)
	// Generate private key
	keyCmd := exec.Command("openssl", "genrsa", "-out", keyPath, "2048")
	if output, err := keyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to generate CA key: %s - %s", err, string(output))
	}

	// Set restrictive permissions on private key
	os.Chmod(keyPath, 0600)

	// Generate self-signed CA certificate
	certCmd := exec.Command("openssl", "req", "-new", "-x509",
		"-key", keyPath,
		"-out", certPath,
		"-days", "3650",
		"-subj", "/CN=Stronghold Root CA/O=Stronghold Security",
	)
	if output, err := certCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to generate CA certificate: %s - %s", err, string(output))
	}

	// Store CA path in config
	m.config.CA.CertPath = certPath
	m.config.CA.KeyPath = keyPath

	return nil
}

// installCA installs the CA certificate to the system trust store
func (m *InstallModel) installCA() error {
	certPath := filepath.Join(ConfigDir(), "ca", "ca.crt")
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return fmt.Errorf("CA certificate not found at %s", certPath)
	}

	return InstallCAToTrustStore(certPath)
}

// InstallCAToTrustStore installs a CA certificate to the system trust store
func InstallCAToTrustStore(certPath string) error {
	switch runtime.GOOS {
	case "linux":
		return installCALinux(certPath)
	case "darwin":
		return installCADarwin(certPath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// installCALinux installs CA to Linux system trust store
func installCALinux(certPath string) error {
	// Determine the correct CA directory based on distro
	caDirs := []string{
		"/usr/local/share/ca-certificates",          // Debian/Ubuntu
		"/etc/pki/ca-trust/source/anchors",          // RHEL/CentOS/Fedora
		"/etc/ca-certificates/trust-source/anchors", // Arch Linux
	}

	var destDir string
	for _, dir := range caDirs {
		if _, err := os.Stat(filepath.Dir(dir)); err == nil {
			destDir = dir
			break
		}
	}

	if destDir == "" {
		return fmt.Errorf("no system CA directory found")
	}

	// Create directory if it doesn't exist
	os.MkdirAll(destDir, 0755)

	// Copy certificate
	destPath := filepath.Join(destDir, "stronghold-ca.crt")
	input, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %w", err)
	}

	if err := os.WriteFile(destPath, input, 0644); err != nil {
		return fmt.Errorf("failed to write CA certificate: %w", err)
	}

	// Update CA certificates
	updateCommands := [][]string{
		{"update-ca-certificates"},     // Debian/Ubuntu
		{"update-ca-trust", "extract"}, // RHEL/CentOS/Fedora
		{"trust", "extract-compat"},    // Arch Linux
	}

	for _, cmd := range updateCommands {
		if _, err := exec.LookPath(cmd[0]); err == nil {
			exec.Command(cmd[0], cmd[1:]...).Run()
			break
		}
	}

	return nil
}

// installCADarwin installs CA to macOS system trust store
func installCADarwin(certPath string) error {
	// Add to system keychain as trusted root
	cmd := exec.Command("security", "add-trusted-cert",
		"-d",              // Add to admin cert store
		"-r", "trustRoot", // Trust as root CA
		"-k", "/Library/Keychains/System.keychain",
		certPath,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to install CA: %s - %s", err, string(output))
	}

	return nil
}

// installProxyBinary installs the proxy binary
func (m *InstallModel) installProxyBinary() error {
	// In a real implementation, this would download or copy the binary
	// For now, we'll just check if it exists or create a placeholder

	destPath := "/usr/local/bin/stronghold-proxy"

	// Check if we're running from source
	if _, err := os.Stat("./cmd/proxy/main.go"); err == nil {
		// Build the proxy
		cmd := exec.Command("go", "build", "-o", destPath, "./cmd/proxy")
		if _, err := cmd.CombinedOutput(); err != nil {
			// Try user-local bin
			userBin := filepath.Join(os.Getenv("HOME"), ".local", "bin")
			os.MkdirAll(userBin, 0755)
			destPath = filepath.Join(userBin, "stronghold-proxy")
			cmd = exec.Command("go", "build", "-o", destPath, "./cmd/proxy")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to build proxy: %s", string(output))
			}
		}
	} else {
		// Check if binary already exists
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			// Create a placeholder script for development
			return fmt.Errorf("proxy binary not found - please build from source")
		}
	}

	return nil
}

// installCLIBinary installs the CLI binary
func (m *InstallModel) installCLIBinary() error {
	// Similar to proxy binary
	destPath := "/usr/local/bin/stronghold"

	if _, err := os.Stat("./cmd/cli/main.go"); err == nil {
		cmd := exec.Command("go", "build", "-o", destPath, "./cmd/cli")
		if _, err := cmd.CombinedOutput(); err != nil {
			userBin := filepath.Join(os.Getenv("HOME"), ".local", "bin")
			os.MkdirAll(userBin, 0755)
			destPath = filepath.Join(userBin, "stronghold")
			cmd = exec.Command("go", "build", "-o", destPath, "./cmd/cli")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to build CLI: %s", string(output))
			}
		}
	}

	return nil
}

// configureService configures the system service
func (m *InstallModel) configureService() error {
	serviceManager := NewServiceManager(m.config)
	return serviceManager.InstallService()
}

// startProxy starts the proxy
func (m *InstallModel) startProxy() error {
	serviceManager := NewServiceManager(m.config)
	return serviceManager.Start()
}

// enableTransparentProxy enables transparent proxying
func (m *InstallModel) enableTransparentProxy() error {
	tp := NewTransparentProxy(m.config)
	if !tp.IsAvailable() {
		return fmt.Errorf("transparent proxy not available on this system")
	}
	return tp.Enable()
}

// RunInit runs the interactive init setup
func RunInit() error {
	model := NewInstallModel()
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}

// RunInitNonInteractive runs a non-interactive init setup
// privateKey: optional hex private key to import for EVM wallet (for pre-funded wallets)
// solanaPrivateKey: optional base58 private key to import for Solana wallet
// accountNumber: optional account number to login to existing account
func RunInitNonInteractive(privateKey, solanaPrivateKey, accountNumber string, skipService bool) error {
	config := DefaultConfig()

	// Check platform
	if !IsSupportedPlatform() {
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	// Check port
	if !config.IsPortAvailable() {
		newPort := FindAvailablePort(config.Proxy.Port + 1)
		if newPort == 0 {
			return fmt.Errorf("no available ports found")
		}
		oldPort := config.Proxy.Port
		config.Proxy.Port = newPort
		fmt.Printf("Port %d in use, using port %d\n", oldPort, newPort)
	}

	// Handle account setup
	apiClient := NewAPIClient(config.API.Endpoint, config.Auth.DeviceToken)
	userID := generateUserID()
	config.Auth.UserID = userID

	if accountNumber != "" {
		// Login to existing account
		fmt.Println("→ Logging into existing account...")
		loginResp, err := apiClient.Login(accountNumber)
		if err != nil {
			return fmt.Errorf("failed to login: %w", err)
		}
		if loginResp.TOTPRequired {
			return fmt.Errorf("TOTP required for new device login. Run interactively to complete verification")
		}
		config.Auth.AccountNumber = loginResp.AccountNumber
		config.Auth.LoggedIn = true

		// Reset local wallet linkage and repopulate from server response.
		config.Wallet.Address = ""
		config.Wallet.SolanaAddress = ""

		evmAddr := loginResp.EVMWalletAddress
		if evmAddr != nil {
			if loginResp.EscrowEnabled {
				walletKey, err := apiClient.GetWalletKey()
				if err != nil {
					// Fallback: use wallet address without key
					config.Wallet.Address = *evmAddr
					config.Wallet.Network = DefaultBlockchain
					fmt.Printf("⚠ Wallet key fetch failed: %v\n", err)
				} else {
					address, err := ImportWallet(userID, DefaultBlockchain, walletKey)
					// Zero the private key after use
					ZeroString(&walletKey)
					if err != nil {
						return fmt.Errorf("failed to import wallet: %w", err)
					}
					config.Wallet.Address = address
					config.Wallet.Network = DefaultBlockchain
					fmt.Printf("✓ Wallet synced: %s\n", address)
				}
			} else {
				config.Wallet.Address = *evmAddr
				config.Wallet.Network = DefaultBlockchain
				fmt.Println("⚠ Wallet not stored on server. Import locally to enable payments.")
			}
		}
		if loginResp.SolanaWalletAddress != nil {
			config.Wallet.SolanaAddress = *loginResp.SolanaWalletAddress
			if config.Wallet.SolanaNetwork == "" {
				config.Wallet.SolanaNetwork = DefaultSolanaNetwork
			}
			fmt.Printf("✓ Solana wallet linked: %s\n", *loginResp.SolanaWalletAddress)
		}
		fmt.Printf("✓ Logged in as %s\n", loginResp.AccountNumber)

		// Register wallet addresses with server (best-effort)
		if err := apiClient.RegisterWalletAddresses(config.Wallet.Address, config.Wallet.SolanaAddress); err == nil {
			if config.Wallet.Address != "" || config.Wallet.SolanaAddress != "" {
				fmt.Println("✓ Wallet addresses registered with server")
			}
		}
	} else {
		// Create new account
		fmt.Println("→ Creating account...")
		req := &CreateAccountRequest{}

		// EVM wallet: import existing key or create new
		if privateKey != "" {
			address, err := ImportWallet(userID, DefaultBlockchain, privateKey)
			if err != nil {
				return fmt.Errorf("failed to import EVM private key: %w", err)
			}
			config.Wallet.Address = address
			config.Wallet.Network = DefaultBlockchain
			req.WalletAddress = &address
			fmt.Printf("✓ EVM wallet imported: %s\n", address)
		} else {
			walletAddress, err := SetupWallet(userID, "base")
			if err != nil {
				fmt.Printf("⚠ EVM wallet setup failed: %v\n", err)
			} else {
				config.Wallet.Address = walletAddress
				config.Wallet.Network = DefaultBlockchain
				req.WalletAddress = &walletAddress
				fmt.Printf("✓ EVM wallet created: %s\n", walletAddress)
			}
		}

		// Solana wallet: import existing key or create new
		if solanaPrivateKey != "" {
			solanaAddr, err := ImportSolanaWallet(userID, DefaultSolanaNetwork, solanaPrivateKey)
			if err != nil {
				return fmt.Errorf("failed to import Solana private key: %w", err)
			}
			config.Wallet.SolanaAddress = solanaAddr
			config.Wallet.SolanaNetwork = DefaultSolanaNetwork
			fmt.Printf("✓ Solana wallet imported: %s\n", solanaAddr)
		} else {
			solanaAddr, solanaErr := SetupSolanaWallet(userID, DefaultSolanaNetwork)
			if solanaErr != nil {
				fmt.Printf("⚠ Solana wallet setup failed: %v\n", solanaErr)
			} else {
				config.Wallet.SolanaAddress = solanaAddr
				config.Wallet.SolanaNetwork = DefaultSolanaNetwork
				fmt.Printf("✓ Solana wallet created: %s\n", solanaAddr)
			}
		}

		resp, err := apiClient.CreateAccount(req)
		if err != nil {
			fmt.Printf("⚠ API unavailable: %v\n", err)
			config.Auth.AccountNumber = generateSimulatedAccountNumber()
		} else {
			config.Auth.AccountNumber = resp.AccountNumber

			// Register wallet addresses with server (best-effort)
			if err := apiClient.RegisterWalletAddresses(config.Wallet.Address, config.Wallet.SolanaAddress); err == nil {
				fmt.Println("✓ Wallet addresses registered with server")
			}
		}
		config.Auth.LoggedIn = true
		fmt.Printf("✓ Account: %s\n", config.Auth.AccountNumber)
	}

	// Save config
	if err := config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if !skipService {
		// Install binaries
		destPath := "/usr/local/bin/stronghold-proxy"
		if _, err := os.Stat("./cmd/proxy/main.go"); err == nil {
			cmd := exec.Command("go", "build", "-o", destPath, "./cmd/proxy")
			if _, err := cmd.CombinedOutput(); err != nil {
				userBin := filepath.Join(os.Getenv("HOME"), ".local", "bin")
				os.MkdirAll(userBin, 0755)
				destPath = filepath.Join(userBin, "stronghold-proxy")
				cmd = exec.Command("go", "build", "-o", destPath, "./cmd/proxy")
				if _, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to build proxy: %w", err)
				}
			}
		}

		// Install service
		serviceManager := NewServiceManager(config)
		if err := serviceManager.InstallService(); err != nil {
			return fmt.Errorf("failed to install service: %w", err)
		}

		// Start proxy
		if err := serviceManager.Start(); err != nil {
			return fmt.Errorf("failed to start proxy: %w", err)
		}

		// Enable transparent proxy
		tp := NewTransparentProxy(config)
		if !tp.IsAvailable() {
			return fmt.Errorf("transparent proxy not available on this system")
		}
		if err := tp.Enable(); err != nil {
			return fmt.Errorf("failed to enable transparent proxy: %w", err)
		}
	}

	config.Installed = true
	config.InstallDate = time.Now().Format(time.RFC3339)
	if err := config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if skipService {
		fmt.Println("\u2713 Initialization complete! (service setup skipped)")
	} else {
		fmt.Println("\u2713 Initialization complete!")
		fmt.Printf("Proxy running on %s (transparent mode)\n", config.GetProxyAddr())
	}

	return nil
}

// Confirm prompts the user for confirmation
func Confirm(prompt string) bool {
	fmt.Print(prompt + " ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// generateUserID generates a unique user ID for wallet creation
func generateUserID() string {
	return fmt.Sprintf("user-%d", time.Now().UnixNano())
}

// generateSimulatedAccountNumber generates a simulated account number for demo
// In production, this would come from the API
func generateSimulatedAccountNumber() string {
	max := big.NewInt(10000)
	parts := make([]string, 4)
	for i := 0; i < 4; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// Fallback should never happen, but don't panic
			n = big.NewInt(0)
		}
		parts[i] = fmt.Sprintf("%04d", n.Int64())
	}
	return fmt.Sprintf("%s-%s-%s-%s", parts[0], parts[1], parts[2], parts[3])
}

// StrongholdUsername returns the username for the stronghold service user
func StrongholdUsername() string {
	if runtime.GOOS == "darwin" {
		return "_stronghold" // macOS convention: underscore prefix for system users
	}
	return "stronghold"
}

// CreateStrongholdUser creates the dedicated system user for the proxy service
func CreateStrongholdUser() error {
	switch runtime.GOOS {
	case "linux":
		return createLinuxUser()
	case "darwin":
		return createDarwinUser()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// createLinuxUser creates the stronghold system user on Linux
func createLinuxUser() error {
	username := StrongholdUsername()

	// Check if user already exists
	if _, err := user.Lookup(username); err == nil {
		return nil // User already exists
	}

	// Create system user (no home directory, no login shell)
	cmd := exec.Command("useradd",
		"--system",
		"--no-create-home",
		"--shell", "/usr/sbin/nologin",
		"--comment", "Stronghold Proxy Service",
		username)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create user: %s - %s", err, string(output))
	}

	return nil
}

// createDarwinUser creates the _stronghold system user on macOS
func createDarwinUser() error {
	username := StrongholdUsername() // "_stronghold"

	// Check if user already exists
	if _, err := user.Lookup(username); err == nil {
		return nil // User already exists
	}

	// Find an available UID in the system range (200-399)
	uid := findAvailableUID()
	if uid == 0 {
		return fmt.Errorf("no available UID found for system user")
	}

	// Create the user using dscl (Directory Service command line)
	commands := [][]string{
		{"dscl", ".", "-create", "/Users/" + username},
		{"dscl", ".", "-create", "/Users/" + username, "UserShell", "/usr/bin/false"},
		{"dscl", ".", "-create", "/Users/" + username, "RealName", "Stronghold Proxy Service"},
		{"dscl", ".", "-create", "/Users/" + username, "UniqueID", fmt.Sprintf("%d", uid)},
		{"dscl", ".", "-create", "/Users/" + username, "PrimaryGroupID", "20"}, // staff group
		{"dscl", ".", "-create", "/Users/" + username, "NFSHomeDirectory", "/var/empty"},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to create user (%v): %s - %s", args, err, string(output))
		}
	}

	return nil
}

// findAvailableUID finds an available UID in the macOS system user range
func findAvailableUID() int {
	// macOS system users typically use UIDs 200-399
	for uid := 399; uid >= 200; uid-- {
		// Check if UID is in use
		cmd := exec.Command("dscl", ".", "-search", "/Users", "UniqueID", fmt.Sprintf("%d", uid))
		output, _ := cmd.Output()
		if len(strings.TrimSpace(string(output))) == 0 {
			return uid
		}
	}
	return 0
}

// GetStrongholdUID returns the UID of the stronghold user
func GetStrongholdUID() (string, error) {
	username := StrongholdUsername()
	u, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("stronghold user not found: run 'stronghold init' first")
	}
	return u.Uid, nil
}

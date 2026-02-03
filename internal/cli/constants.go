package cli

import "time"

const (
	// Private key validation (always validated after stripping 0x prefix)
	PrivateKeyHexLength = 64

	// Input field widths
	PortInputWidth          = 5
	APIInputWidth           = 50
	PrivateKeyInputWidth    = 70
	AccountNumberInputWidth = 25

	// Input character limits
	PortInputCharLimit          = 5
	APIInputCharLimit           = 100
	PrivateKeyInputCharLimit    = 66 // 64 hex chars + optional 0x prefix
	AccountNumberInputCharLimit = 19 // XXXX-XXXX-XXXX-XXXX

	// Defaults
	DefaultProxyPort   = 8402
	DefaultAPIEndpoint = "https://api.stronghold.security"
	DefaultBlockchain  = "base"

	// Retries
	MaxAccountNumberRetries = 10

	// File size limits
	MaxKeyFileSize = 1024 // 1 KB

	// Account setup choices
	AccountChoiceCreate          = 0
	AccountChoiceCreateWithKey   = 1
	AccountChoiceExistingAccount = 2
	AccountChoiceSkip            = 3
	MaxAccountChoices            = 4

	// UI timing delays
	SystemCheckDelay = 500 * time.Millisecond
	PostCheckDelay   = 300 * time.Millisecond
	InstallStepDelay = 200 * time.Millisecond
)

package usdc

const defaultUSDDecimals = 6

// chainDecimals defines the USDC token decimal places per chain.
// This is the single source of truth — never hardcode decimals elsewhere.
var chainDecimals = map[string]int{
	"base":          defaultUSDDecimals,
	"base-sepolia":  defaultUSDDecimals,
	"solana":        defaultUSDDecimals,
	"solana-devnet": defaultUSDDecimals,
}

// DecimalsForChain returns USDC decimals for a given chain.
func DecimalsForChain(chain string) int {
	if decimals, ok := chainDecimals[chain]; ok {
		return decimals
	}
	return defaultUSDDecimals
}

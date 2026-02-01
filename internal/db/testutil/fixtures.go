package testutil

import (
	"fmt"
	"time"
)

// RandomWalletAddress generates a random Ethereum wallet address for testing
func RandomWalletAddress() string {
	return fmt.Sprintf("0x%040x", time.Now().UnixNano())
}

// RandomAccountNumber generates a random account number in the expected format
func RandomAccountNumber() string {
	return fmt.Sprintf("%04d-%04d-%04d-%04d",
		time.Now().UnixNano()%10000,
		(time.Now().UnixNano()/10000)%10000,
		(time.Now().UnixNano()/100000000)%10000,
		(time.Now().UnixNano()/1000000000000)%10000,
	)
}

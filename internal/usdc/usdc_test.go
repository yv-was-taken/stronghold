package usdc

import (
	"encoding/json"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromFloat(t *testing.T) {
	tests := []struct {
		input    float64
		expected MicroUSDC
	}{
		{0, 0},
		{0.000001, 1},
		{0.001, 1_000},
		{0.01, 10_000},
		{0.09, 90_000},
		{0.19, 190_000},
		{1.0, 1_000_000},
		{1.25, 1_250_000},
		{100.0, 100_000_000},
		{0.123456, 123_456},
		{99999.999999, 99_999_999_999},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			result := FromFloat(tc.input)
			assert.Equal(t, tc.expected, result, "FromFloat(%v)", tc.input)
		})
	}
}

func TestFloat(t *testing.T) {
	tests := []struct {
		input    MicroUSDC
		expected float64
	}{
		{0, 0},
		{1, 0.000001},
		{1_000, 0.001},
		{1_000_000, 1.0},
		{1_250_000, 1.25},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			result := tc.input.Float()
			assert.InDelta(t, tc.expected, result, 1e-9)
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		input    MicroUSDC
		expected string
	}{
		{0, "0.00"},
		{1, "0.000001"},
		{100, "0.0001"},
		{1_000, "0.001"},
		{10_000, "0.01"},
		{100_000, "0.10"},
		{1_000_000, "1.00"},
		{1_250_000, "1.25"},
		{1_250_001, "1.250001"},
		{10_000_000, "10.00"},
		{99_999_999_999, "99999.999999"},
		{-1_250_000, "-1.25"},
		{MicroUSDC(math.MinInt64), "-9223372036854.775808"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.input.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		input    MicroUSDC
		expected string
	}{
		{0, `"0"`},
		{1_000, `"1000"`},
		{1_250_000, `"1250000"`},
		{-500, `"-500"`},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			data, err := json.Marshal(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, string(data))
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected MicroUSDC
	}{
		{"string", `"1250000"`, 1_250_000},
		{"number", `1250000`, 1_250_000},
		{"zero string", `"0"`, 0},
		{"zero number", `0`, 0},
		{"negative string", `"-500"`, -500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m MicroUSDC
			err := json.Unmarshal([]byte(tc.input), &m)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, m)
		})
	}
}

func TestUnmarshalJSON_Error(t *testing.T) {
	var m MicroUSDC
	err := json.Unmarshal([]byte(`"not-a-number"`), &m)
	assert.Error(t, err)
}

func TestMarshalJSON_InStruct(t *testing.T) {
	type Example struct {
		Balance MicroUSDC `json:"balance_usdc"`
	}

	e := Example{Balance: 1_250_000}
	data, err := json.Marshal(e)
	require.NoError(t, err)
	assert.Equal(t, `{"balance_usdc":"1250000"}`, string(data))

	var decoded Example
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, MicroUSDC(1_250_000), decoded.Balance)
}

func TestToBigInt_Base(t *testing.T) {
	// Base has 6 decimals, same as MicroUSDC scale — direct conversion
	tests := []struct {
		input    MicroUSDC
		expected string
	}{
		{0, "0"},
		{1_000, "1000"},
		{1_000_000, "1000000"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.input.ToBigInt("base")
			assert.Equal(t, tc.expected, result.String())
		})
	}
}

func TestToBigInt_FromBigInt_RoundTrip(t *testing.T) {
	chains := []string{"base", "base-sepolia", "solana", "solana-devnet"}
	values := []MicroUSDC{0, 1, 1_000, 1_000_000, 99_999_999_999, -1_250_000, MicroUSDC(math.MaxInt64)}

	for _, chain := range chains {
		for _, v := range values {
			t.Run("", func(t *testing.T) {
				bi := v.ToBigInt(chain)
				back := FromBigInt(bi, chain)
				assert.Equal(t, v, back, "round-trip failed for chain=%s value=%d", chain, v)
			})
		}
	}
}

func TestFromBigInt(t *testing.T) {
	// All current chains have 6 decimals, so it's a direct conversion
	bi := big.NewInt(1_250_000)
	result := FromBigInt(bi, "base")
	assert.Equal(t, MicroUSDC(1_250_000), result)
}

func TestFromBigInt_ClampOnOverflow(t *testing.T) {
	tooBig := new(big.Int).Add(big.NewInt(math.MaxInt64), big.NewInt(1))
	tooSmall := new(big.Int).Sub(big.NewInt(math.MinInt64), big.NewInt(1))

	assert.Equal(t, MicroUSDC(math.MaxInt64), FromBigInt(tooBig, "base"))
	assert.Equal(t, MicroUSDC(math.MinInt64), FromBigInt(tooSmall, "base"))
}

func TestScaleForChain(t *testing.T) {
	assert.Equal(t, int64(1_000_000), ScaleForChain("base"))
	assert.Equal(t, int64(1_000_000), ScaleForChain("solana"))
	assert.Equal(t, int64(1_000_000), ScaleForChain("unknown")) // defaults to 6
}

func TestFromFloat_RoundTrip(t *testing.T) {
	// Verify that FromFloat -> Float round-trips correctly for common values
	values := []float64{0, 0.001, 0.01, 0.10, 1.00, 1.25, 100.00}
	for _, v := range values {
		m := FromFloat(v)
		assert.InDelta(t, v, m.Float(), 1e-7, "round-trip for %v", v)
	}
}

func TestMicroUSDCValue(t *testing.T) {
	value, err := MicroUSDC(1_250_000).Value()
	require.NoError(t, err)
	assert.Equal(t, int64(1_250_000), value)
}

func TestMicroUSDCScan(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  MicroUSDC
		shouldErr bool
	}{
		{name: "int64", input: int64(1250000), expected: 1_250_000},
		{name: "int32", input: int32(1250000), expected: 1_250_000},
		{name: "int", input: int(1250000), expected: 1_250_000},
		{name: "string", input: "1250000", expected: 1_250_000},
		{name: "bytes", input: []byte("1250000"), expected: 1_250_000},
		{name: "float64 integer", input: float64(1250000), expected: 1_250_000},
		{name: "nil", input: nil, expected: 0},
		{name: "float64 fractional", input: 1.25, shouldErr: true},
		{name: "bad string", input: "not-a-number", shouldErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m MicroUSDC
			err := m.Scan(tc.input)
			if tc.shouldErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, m)
		})
	}
}

func TestMicroUSDCScan_NilReceiver(t *testing.T) {
	var m *MicroUSDC
	err := m.Scan(int64(1))
	assert.Error(t, err)
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func executeRoot(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestWalletReplace_RequiresChainArg(t *testing.T) {
	_, _, err := executeRoot(t, "wallet", "replace", "--yes")
	if err == nil {
		t.Fatal("expected error when chain arg is omitted")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s), received 0") {
		t.Fatalf("expected arg validation error, got: %v", err)
	}
}

func TestWalletReplace_AcceptsExplicitChains(t *testing.T) {
	for _, chain := range []string{"evm", "solana"} {
		_, _, err := executeRoot(t, "wallet", "replace", chain, "--yes")
		if err == nil {
			t.Fatalf("expected runtime error (no config/login) for chain %q, got nil", chain)
		}
		if strings.Contains(err.Error(), "accepts 1 arg(s)") {
			t.Fatalf("unexpected arg validation error for chain %q: %v", chain, err)
		}
	}
}

func TestWalletReplace_HelpExamplesIncludeChainArg(t *testing.T) {
	stdout, stderr, err := executeRoot(t, "help", "wallet", "replace")
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	help := stdout + "\n" + stderr
	required := []string{
		"stronghold wallet replace evm --yes",
		"stronghold wallet replace solana --yes",
		"stronghold wallet replace evm --file /path/to/key.txt",
		"stronghold wallet replace solana --file /path/to/solana-key.txt",
	}
	for _, want := range required {
		if !strings.Contains(help, want) {
			t.Fatalf("help text missing required example: %q", want)
		}
	}

	banned := []string{
		"stronghold wallet replace --yes",
		"stronghold wallet replace --file /path/to/key.txt",
	}
	for _, bad := range banned {
		if strings.Contains(help, bad) {
			t.Fatalf("help text still includes invalid example: %q", bad)
		}
	}
}

func TestDocs_DoNotUseDeprecatedWalletReplaceForms(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	files := []string{
		"README.md",
		"web/public/llms.txt",
		"web/public/llms-full.txt",
	}

	var all strings.Builder
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatalf("failed to read %s: %v", rel, err)
		}
		all.Write(data)
		all.WriteString("\n")
	}

	content := all.String()
	for _, bad := range []string{
		"wallet replace --chain",
		"wallet replace --yes",
		"wallet replace --file /path/to/key.txt",
		"wallet replace --file /path/to/evm-key.txt",
	} {
		if strings.Contains(content, bad) {
			t.Fatalf("docs still contain deprecated/invalid form: %q", bad)
		}
	}

	for _, want := range []string{
		"wallet replace evm --yes",
		"wallet replace solana --yes",
		"wallet replace evm --file",
		"wallet replace solana --file",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("docs missing required form: %q", want)
		}
	}
}

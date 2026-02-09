#!/bin/sh
# Substitute RPC URL env vars into config template (x402-rs only interpolates signer keys natively)
sed "s|\$RPC_URL_BASE_SEPOLIA|${RPC_URL_BASE_SEPOLIA:-https://sepolia.base.org}|g;s|\$RPC_URL_BASE|${RPC_URL_BASE:-https://mainnet.base.org}|g" /app/config.json > /tmp/config.json
exec x402-facilitator --config /tmp/config.json

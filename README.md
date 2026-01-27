# Polymarket Trading Bot

Four strategies for Polymarket prediction markets.

## Strategies

| Strategy          | Command          | Description                                           |
| ----------------- | ---------------- | ----------------------------------------------------- |
| **Weather**       | `make weather`   | Exploit forecast/market price divergence. Daily edge. |
| **Black Swan**    | `make blackswan` | Buy low-probability events (1-10¢) for 10x-1000x.     |
| **Sports**        | `make sports`    | Snipe NFL/NBA when outcome becomes certain.           |
| **15-Min Crypto** | `make run`       | Snipe crypto up/down in final seconds. Low volume.    |

## Quick Start

```bash
cp .env.example .env   # Add your credentials
make build
make weather-dry       # Test weather sniper
make approve           # One-time USDC approval (before live trading)
```

## Required Credentials

Get CLOB API keys: [polymarket.com/settings?tab=builder](https://polymarket.com/settings?tab=builder)

```env
PRIVATE_KEY=0x...
CLOB_API_KEY=...
CLOB_SECRET=...
CLOB_PASSPHRASE=...
```

## Proxy Wallet Setup

If you deposited via Polymarket UI, funds are in a **proxy wallet**:

```env
PROXY_WALLET_ADDRESS=0x...  # Find in Polymarket settings

# Signature type (required for proxy wallet)
# 1 = Email/Google login (Magic Link)
# 2 = Browser wallet (MetaMask) [DEFAULT]
SIGNATURE_TYPE=2
```

Run `make balance` to verify.

## Commands

```bash
make build         # Build all
make balance       # Check balances
make approve       # USDC approval (one-time)

# Live trading
make weather       # Weather sniper
make blackswan     # Black Swan hunter
make sports        # Sports sniper
make run           # 15-min crypto

# Dry run (test mode)
make weather-dry
make blackswan-dry
make sports-dry
make run-dry
```

## Go Live

```env
DRY_RUN=false
```

## License

MIT


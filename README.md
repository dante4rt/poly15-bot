# Polymarket Trading Bot

Two strategies for Polymarket prediction markets.

## Strategies

| Strategy          | Command          | Description                                                              |
| ----------------- | ---------------- | ------------------------------------------------------------------------ |
| **Black Swan**    | `make blackswan` | Buy low-probability events (1-10¢) with limit orders. Power-law returns. |
| **15-Min Sniper** | `make run`       | Snipe crypto up/down markets in final seconds. Low liquidity issue.      |

## Quick Start

```bash
cp .env.example .env   # Add your credentials
make build
make blackswan-dry     # Test Black Swan
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

## Black Swan Config

```env
MAX_POSITION_SIZE=15        # Bankroll ($)
BLACKSWAN_MAX_PRICE=0.10    # Max 10¢
BLACKSWAN_BET_PERCENT=0.05  # 5% per bet
BLACKSWAN_MAX_POSITIONS=10  # Max open orders
BLACKSWAN_MAX_EXPOSURE=10   # Max $ at risk
BLACKSWAN_BID_DISCOUNT=0.25 # Bid 25% below market
```

## Commands

```bash
make build         # Build all
make approve       # USDC approval (one-time)
make blackswan     # Black Swan (live)
make blackswan-dry # Black Swan (test)
make run           # 15-min sniper (live)
make run-dry       # 15-min sniper (test)
```

## Go Live

```env
DRY_RUN=false
```

## License

MIT

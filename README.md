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
make run-dry           # Test 15-min sniper
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
make blackswan     # Run Black Swan (live)
make blackswan-dry # Run Black Swan (test)
make run           # Run 15-min sniper (live)
make run-dry       # Run 15-min sniper (test)
make scan          # Find active markets
```

## Go Live

```env
DRY_RUN=false
```

## License

MIT

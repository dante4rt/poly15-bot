# Polymarket Trading Bot

Three strategies for Polymarket prediction markets.

## Strategies

| Strategy          | Command          | Description                                                 |
| ----------------- | ---------------- | ----------------------------------------------------------- |
| **Black Swan**    | `make blackswan` | Buy low-probability events (1-10¢) with GTC limit orders.   |
| **Sports**        | `make sports`    | Snipe NFL/NBA markets when outcome becomes certain.         |
| **15-Min Crypto** | `make run`       | Snipe crypto up/down markets in final seconds. Low volume.  |

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

## Proxy Wallet (if deposited via UI)

If you deposited USDC through Polymarket's web UI, your funds are in a **proxy wallet** (Gnosis Safe), not your EOA. Find your proxy address in Polymarket settings:

```env
PROXY_WALLET_ADDRESS=0x...  # Your Polymarket proxy wallet
```

Run `make balance` to check balances and positions.

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
make balance       # Check USDC balance and positions
make approve       # USDC approval (one-time)
make blackswan     # Black Swan (live)
make blackswan-dry # Black Swan (dry run)
make sports        # Sports sniper (live)
make sports-dry    # Sports sniper (dry run)
make run           # 15-min crypto (live)
make run-dry       # 15-min crypto (dry run)
```

## Go Live

```env
DRY_RUN=false
```

## License

MIT

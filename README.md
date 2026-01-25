# Polymarket 15-Minute Sniper Bot

Latency arbitrage bot for Polymarket's 15-minute BTC/ETH up/down markets. Snipes winning outcomes in the final seconds before resolution.

## Strategy

When a 15-min market has <N seconds remaining (configurable via `TRIGGER_SECONDS`) and the outcome is clear (price > 65%), buy the winning side at market ask to collect $1.00 at settlement.

**Key insight**: The bot uses Gamma API's indicative prices (`bestBid`/`bestAsk`) to determine the winning side, since CLOB order books often have extreme spreads (0.01/0.99) with no liquidity near the actual price. CLOB is only used for order execution.

## Quick Start

```bash
# 1. Clone and setup
git clone https://github.com/dante4rt/poly15-bot.git
cd poly15-bot
cp .env.example .env

# 2. Get API credentials (see below)
# 3. Fill .env with your keys

# 4. Run
make build
make run-dry  # Test mode first
```

## Getting API Credentials

### Step 1: Get Private Key
Export from your Polygon wallet (MetaMask, etc.)

### Step 2: Generate CLOB API Keys

```bash
pip install py-clob-client
```

```python
from py_clob_client.client import ClobClient

client = ClobClient(
    host="https://clob.polymarket.com",
    key="YOUR_PRIVATE_KEY",
    chain_id=137
)

creds = client.create_or_derive_api_creds()
print(creds)  # Save these to .env
```

Or via Polymarket UI: [polymarket.com/settings?tab=builder](https://polymarket.com/settings?tab=builder)

### Step 3: Telegram (Optional)
1. Create bot via [@BotFather](https://t.me/BotFather)
2. Get chat ID via [@userinfobot](https://t.me/userinfobot)

## Configuration

```env
# Required - Wallet
PRIVATE_KEY=0x...

# Required - CLOB API (generate via py-clob-client)
CLOB_API_KEY=...
CLOB_SECRET=...
CLOB_PASSPHRASE=...

# Optional - Telegram alerts
TELEGRAM_BOT_TOKEN=...
TELEGRAM_CHAT_ID=...

# Trading Parameters
DRY_RUN=true              # Start with true to test
MAX_POSITION_SIZE=10      # Max $ per trade
SNIPE_PRICE=0.99          # Max buy price (0.99 = $0.99)
TRIGGER_SECONDS=1         # How many seconds before market end to trigger snipe
MIN_LIQUIDITY=5           # Minimum $ available at ask to execute trade

# Network (usually don't need to change)
POLYGON_CHAIN_ID=137
POLYGON_RPC_URL=https://polygon-rpc.com
```

### Parameter Explanation

| Parameter | Default | Description |
|-----------|---------|-------------|
| `TRIGGER_SECONDS` | 1 | Snipe triggers when market has this many seconds left |
| `MIN_LIQUIDITY` | 5 | Skip if available liquidity at winning ask is below this |
| `SNIPE_PRICE` | 0.99 | Maximum price to pay for winning outcome |
| `MAX_POSITION_SIZE` | 10 | Max dollars per trade |

## Commands

```bash
make build          # Build binaries
make scan           # Find active markets
make run-dry        # Run in test mode
make run            # Run live (DRY_RUN=false)
make test           # Run tests
```

## Docker

```bash
make docker-build
make docker-run     # Runs sniper
make docker-logs    # View logs
make docker-stop
```

## Architecture

```
cmd/
  scanner/    # Find active markets
  sniper/     # Main bot
  approve/    # USDC approval
internal/
  config/     # Environment loading
  wallet/     # EIP-712 signing
  gamma/      # Market discovery
  clob/       # REST + WebSocket
  strategy/   # Sniper logic
  telegram/   # Notifications
```

## How It Works

1. **Market Discovery**: Queries Gamma API for active 15-min up/down markets (BTC, ETH, SOL, XRP). Markets use slug pattern `{asset}-updown-15m-{startTimestamp}` where the timestamp is the START time, and `endDate` = start + 15 minutes.

2. **Price Monitoring**: Uses Gamma API's `outcomePrices` and `bestBid`/`bestAsk` for accurate pricing. CLOB order books often show 0.01/0.99 spreads with no real liquidity.

3. **Trigger**: When `time_remaining <= TRIGGER_SECONDS` and winning side has price > 65%, the bot analyzes:
   - Is there enough liquidity at the winning ask? (>= `MIN_LIQUIDITY`)
   - Is the ask price acceptable? (<= `SNIPE_PRICE`)

4. **Execution**: Submits Fill-or-Kill (FOK) order to CLOB. FOK ensures full fill or no fill.

5. **Settlement**: Winning outcome pays $1.00 per share at market resolution.

## Risk Controls

- Winner confidence threshold: 65%
- Max spread: 5% (ask - bid)
- Min liquidity: $5 (configurable via `MIN_LIQUIDITY`)
- FOK orders only (no partial fills)
- DRY_RUN mode for testing

## Disclaimer

Trading crypto derivatives carries risk. Test thoroughly in DRY_RUN mode. Not financial advice.

## License

MIT

# Polymarket 15-Minute Sniper Bot

Latency arbitrage bot for Polymarket's 15-minute BTC/ETH up/down markets. Snipes winning outcomes in the final seconds before resolution.

## Strategy

When a 15-min market has <1 second remaining and outcome is known (price > 0.65), buy winning side at market ask to collect $1.00 at settlement.

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
PRIVATE_KEY=0x...
CLOB_API_KEY=...
CLOB_SECRET=...
CLOB_PASSPHRASE=...
TELEGRAM_BOT_TOKEN=...    # Optional
TELEGRAM_CHAT_ID=...      # Optional
DRY_RUN=true              # Start with true
MAX_POSITION_SIZE=10      # Max $ per trade
SNIPE_PRICE=0.99          # Max buy price
TRIGGER_SECONDS=1         # Seconds before end
```

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

## Risk Controls

- Winner confidence threshold: 65%
- Max spread: 5%
- Min liquidity: $50
- Daily loss limit: $50 (configurable)
- FOK orders only (no partial fills)

## Disclaimer

Trading crypto derivatives carries risk. Test thoroughly in DRY_RUN mode. Not financial advice.

## License

MIT

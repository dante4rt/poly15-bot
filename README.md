# Polymarket 15-Minute Sniper Bot

Latency arbitrage bot for Polymarket's 15-minute crypto up/down markets. Snipes winning outcomes in the final seconds before resolution.

> [!NOTE]
> The bot uses **Gamma API prices** to determine winners, not CLOB order books. CLOB books often show extreme 0.01/0.99 spreads with no real liquidity. CLOB is only used for order execution.

## Quick Start

```bash
git clone https://github.com/dante4rt/poly15-bot.git
cd poly15-bot
cp .env.example .env
# Fill .env with your credentials (see Setup below)
make build
make run-dry  # Test mode first
```

## Setup

### 1. Private Key
Export from your Polygon wallet (MetaMask, etc.)

### 2. CLOB API Keys

**Option A** - Via Python:
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
print(client.create_or_derive_api_creds())
```

**Option B** - Via UI: [polymarket.com/settings?tab=builder](https://polymarket.com/settings?tab=builder)

### 3. Telegram (Optional)
1. Create bot via [@BotFather](https://t.me/BotFather)
2. Get chat ID via [@userinfobot](https://t.me/userinfobot)

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PRIVATE_KEY` | - | Polygon wallet private key (required) |
| `CLOB_API_KEY` | - | CLOB API key (required) |
| `CLOB_SECRET` | - | CLOB secret (required) |
| `CLOB_PASSPHRASE` | - | CLOB passphrase (required) |
| `DRY_RUN` | `true` | Test mode - logs but doesn't trade |
| `MAX_POSITION_SIZE` | `10` | Max USD per trade |
| `SNIPE_PRICE` | `0.99` | Max price to pay for winning outcome |
| `TRIGGER_SECONDS` | `1` | Seconds before market end to trigger |
| `MIN_LIQUIDITY` | `5` | Min USD liquidity required at ask |
| `TELEGRAM_BOT_TOKEN` | - | Telegram bot token (optional) |
| `TELEGRAM_CHAT_ID` | - | Telegram chat ID (optional) |

## Usage

```bash
make build          # Build binaries
make run-dry        # Test mode (DRY_RUN=true)
make run            # Live mode (DRY_RUN=false)
make scan           # Find active markets
```

**Docker:**
```bash
make docker-build && make docker-run
make docker-logs    # View logs
make docker-stop    # Stop container
```

## Strategy

```
1. Discover    Gamma API → Find 15-min up/down markets (BTC, ETH, SOL, XRP)
2. Monitor     Track Gamma prices until TRIGGER_SECONDS before end
3. Analyze     Winner = side with price > 50% (UP side only due to liquidity)
4. Execute     FOK order at best ask if price <= SNIPE_PRICE
5. Settle      Winning outcome pays $1.00 per share
```

> [!IMPORTANT]
> **UP-only trading**: DOWN tokens typically have no liquidity (ask=0.99). The bot only trades when UP/YES is winning.

## Architecture

```
cmd/
  sniper/     Main bot
  scanner/    Market discovery tool
  approve/    USDC approval utility
internal/
  strategy/   Core sniper logic
  gamma/      Market discovery (gamma-api.polymarket.com)
  clob/       Order execution (REST + WebSocket)
  wallet/     EIP-712 signing
  config/     Environment loading
  telegram/   Notifications
```

## Risk Controls

| Control | Value |
|---------|-------|
| Winner confidence | > 50% |
| Max entry price | $0.99 |
| Min liquidity | $5 |
| Order type | FOK (no partial fills) |
| Daily loss limit | $50 |
| Max loss per trade | $5 |

> [!WARNING]
> Always test with `DRY_RUN=true` first. Trading crypto derivatives carries risk. Not financial advice.

## License

MIT

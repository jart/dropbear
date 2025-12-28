#!/bin/bash
# Download minute candle data for leveraged ETF margin arbitrage research
#
# These ETFs have 30% maintenance margin instead of the proper 75%,
# enabling up to 40x effective day trading leverage on 3x products.
#
# Run: ./scripts/download-leverage.sh
#
# Data goes to: ~/equitydata/minutes/<SYMBOL>/<YYYY-MM>

set -e
cd "$(dirname "$0")/.."

# Tier 1: 3x MicroSectors ETNs (30% margin, should be 75%)
# These are the crown jewels - 3x leverage with only 30% margin requirement
TIER1="FNGU BNKU NRGU BNKD NRGD"

# Tier 2: 2x Single-Stock ETFs on mega-caps (30% margin)
# High volatility underlyings with 2x leverage
TIER2="NVDB PLTA TSLI QQQU GOU"

# Tier 3: Other interesting 2x products (30% margin)
TIER3="DAMD CONX ORCU ARMU"

# Control group: Properly margined 3x ETFs (75% margin)
# For comparison - these have correct margin requirements
CONTROL="TQQQ SOXL TECL SPXL"

# Underlying assets for correlation analysis
UNDERLYING="QQQ SPY NVDA AAPL TSLA PLTR AMD GOOGL"

echo "========================================"
echo "Leveraged ETF Data Download"
echo "========================================"
echo ""
echo "Tier 1 (3x @ 30% MM): $TIER1"
echo "Tier 2 (2x @ 30% MM): $TIER2"
echo "Tier 3 (2x @ 30% MM): $TIER3"
echo "Control (3x @ 75% MM): $CONTROL"
echo "Underlying: $UNDERLYING"
echo ""
echo "Total symbols: $(echo $TIER1 $TIER2 $TIER3 $CONTROL $UNDERLYING | wc -w | tr -d ' ')"
echo ""
echo "This will download all available historical minute data."
echo "Expect this to take 30+ minutes depending on API rate limits."
echo ""
echo "Press Ctrl+C to cancel, or wait 5 seconds to continue..."
sleep 5

echo ""
echo "Starting download..."
echo ""

go run ./cmd/download.alpaca -workers=20 \
    $TIER1 $TIER2 $TIER3 $CONTROL $UNDERLYING

echo ""
echo "========================================"
echo "Download complete!"
echo "========================================"
echo ""
echo "Data location: ~/equitydata/minutes/"
echo ""
echo "Next steps:"
echo "  1. Run analysis: go run ./cmd/leverage.analyze"
echo "  2. Review volatility, decay, and liquidity metrics"
echo "  3. Design day trading strategy based on findings"

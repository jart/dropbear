#!/usr/bin/env python

import os
import datetime
import databento as db

with open(os.path.expanduser("~/.databento.key")) as f:
    api_key = f.read().strip()

# Create a historical client
client = db.Historical(api_key)

# Set parameters
dataset = "OPRA.PILLAR"
parent_symbol = "SPXW"  # SPX Weekly Options
date = datetime.date(2025, 9, 10)

# Download definition data for a parent symbol
def_data = client.timeseries.get_range(
    dataset=dataset,
    schema="definition",
    symbols=f"{parent_symbol}.OPT",
    stype_in="parent",
    start=date,
)

# Convert to DataFrame and filter for 0DTE
def_df = def_data.to_df()
def_df = def_df[def_df["expiration"].dt.date == date]

# Print out raw_symbols, expiration, and strike price
print(def_df[["raw_symbol", "expiration", "strike_price"]])

# Download some TCBBO data with these symbols
tcbbo_data = client.timeseries.get_range(
    dataset=dataset,
    schema="tcbbo",
    symbols=def_df["raw_symbol"].to_list(),
    start=date,
    limit=10000,
)

# Convert to DataFrame
tcbbo_df = tcbbo_data.to_df()

# Print TCBBO DataFrame
print(tcbbo_df)


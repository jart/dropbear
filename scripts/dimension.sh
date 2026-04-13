#!/bin/bash
# Resize an EC2 instance by core count.
# Usage: scripts/dimension.sh <name> <cores>

set -e

declare -A INSTANCES
INSTANCES[penny]=i-04790977c160f6adb

declare -A Z1D_CORES
Z1D_CORES[1]=z1d.large
Z1D_CORES[2]=z1d.xlarge
Z1D_CORES[4]=z1d.2xlarge
Z1D_CORES[6]=z1d.3xlarge
Z1D_CORES[12]=z1d.6xlarge
Z1D_CORES[24]=z1d.12xlarge

declare -A Z1D_RAM
Z1D_RAM[1]="16 GB"
Z1D_RAM[2]="32 GB"
Z1D_RAM[4]="64 GB"
Z1D_RAM[6]="96 GB"
Z1D_RAM[12]="192 GB"
Z1D_RAM[24]="384 GB"

if [ $# -ne 2 ]; then
    echo "usage: scripts/dimension.sh <name> <cores>" >&2
    echo "       cores: 1, 2, 4, 6, 12, 24" >&2
    exit 1
fi

NAME=$1
CORES=$2
INSTANCE_ID=${INSTANCES[$NAME]}
NEW_TYPE=${Z1D_CORES[$CORES]}

if [ -z "$INSTANCE_ID" ]; then
    echo "error: unknown instance '$NAME'" >&2
    echo "known: ${!INSTANCES[*]}" >&2
    exit 1
fi

if [ -z "$NEW_TYPE" ]; then
    echo "error: invalid core count '$CORES'" >&2
    echo "valid: ${!Z1D_CORES[*]}" >&2
    exit 1
fi

# Get current instance type
CURRENT_TYPE=$(aws ec2 describe-instances \
    --instance-ids "$INSTANCE_ID" \
    --query 'Reservations[0].Instances[0].InstanceType' \
    --output text)

if [ "$CURRENT_TYPE" = "$NEW_TYPE" ]; then
    echo "$NAME: already $NEW_TYPE ($CORES cores, ${Z1D_RAM[$CORES]})"
    exit 0
fi

echo "$NAME: $CURRENT_TYPE → $NEW_TYPE ($CORES cores, ${Z1D_RAM[$CORES]})"

# Stop
echo "stopping $NAME..."
aws ec2 stop-instances --instance-ids "$INSTANCE_ID" --output text >/dev/null
aws ec2 wait instance-stopped --instance-ids "$INSTANCE_ID"
echo "stopped"

# Resize
aws ec2 modify-instance-attribute \
    --instance-id "$INSTANCE_ID" \
    --instance-type "$NEW_TYPE"

# Start
echo "starting $NAME..."
aws ec2 start-instances --instance-ids "$INSTANCE_ID" --output text >/dev/null
aws ec2 wait instance-running --instance-ids "$INSTANCE_ID"

# Get new IP (may change on stop/start)
NEW_IP=$(aws ec2 describe-instances \
    --instance-ids "$INSTANCE_ID" \
    --query 'Reservations[0].Instances[0].PublicIpAddress' \
    --output text)

echo "$NAME is up at $NEW_IP"

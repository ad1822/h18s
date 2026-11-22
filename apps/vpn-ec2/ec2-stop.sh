#!/bin/sh
set -eu
. /scripts/common.sh

if [ "$STATE" = "stopped" ]; then
  echo "==> Already stopped, nothing to do"
  exit 0
fi

echo "==> Stopping instance"
aws ec2 stop-instances --instance-ids "$INSTANCE_ID" >/dev/null
aws ec2 wait instance-stopped --instance-ids "$INSTANCE_ID"
echo "    stopped"

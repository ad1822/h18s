#!/bin/sh
set -eu
. /scripts/common.sh

# stop-instances needs only ec2:StopInstances (no Describe), and it's a no-op if
# the instance is already stopped/stopping.
echo "==> Stopping $INSTANCE_ID"
aws ec2 stop-instances --instance-ids "$INSTANCE_ID" \
  --query 'StoppingInstances[0].[PreviousState.Name,CurrentState.Name]' --output text

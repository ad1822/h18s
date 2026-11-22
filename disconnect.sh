#!/usr/bin/env bash
set -euo pipefail

NAME_TAG="HeadScale" # same Name tag as the start script

ROLE_ARN="arn:aws:iam::861341510111:role/homelab"

unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_PROFILE

CREDS=$(AWS_PROFILE=ad aws sts assume-role \
  --role-arn "$ROLE_ARN" --role-session-name homelab-session \
  --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' --output text)
read -r AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN <<<"$CREDS"
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN

echo "==> Stopping local tailscale"
sudo systemctl stop tailscaled

echo "==> Finding instance"
INSTANCE_ID=$(aws ec2 describe-instances \
  --filters "Name=tag:Name,Values=${NAME_TAG}" \
  "Name=instance-state-name,Values=pending,running,stopping,stopped" \
  --query 'Reservations[0].Instances[0].InstanceId' --output text)
if [[ -z "$INSTANCE_ID" || "$INSTANCE_ID" == "None" ]]; then
  echo "instance with Name=${NAME_TAG} not found"
  exit 1
fi
echo "    $INSTANCE_ID"

STATE=$(aws ec2 describe-instances --instance-ids "$INSTANCE_ID" \
  --query 'Reservations[0].Instances[0].State.Name' --output text)

if [[ "$STATE" == "stopped" ]]; then
  echo "==> Already stopped, nothing to do"
  exit 0
fi

echo "==> Stopping instance (was: $STATE)"
aws ec2 stop-instances --instance-ids "$INSTANCE_ID" >/dev/null

echo "==> Waiting for stopped state"
aws ec2 wait instance-stopped --instance-ids "$INSTANCE_ID"
echo "    stopped"

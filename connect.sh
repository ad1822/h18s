#!/usr/bin/env bash
set -euo pipefail

DOMAIN="h18s"
FQDN="${DOMAIN}.duckdns.org"
NAME_TAG="HeadScale"                    # your instance's Name tag
DUCKDNS_TOKEN="$(cat ~/.duckdns_token)" # chmod 600 this file
ROLE_ARN="arn:aws:iam::861341510111:role/homelab"

unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_PROFILE

CREDS=$(AWS_PROFILE=ad aws sts assume-role \
  --role-arn "$ROLE_ARN" --role-session-name homelab-session \
  --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' --output text)
read -r AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN <<<"$CREDS"
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN

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
echo "==> Current state: $STATE"

if [[ "$STATE" == "stopping" ]]; then
  echo "==> Waiting for it to finish stopping"
  aws ec2 wait instance-stopped --instance-ids "$INSTANCE_ID"
  STATE="stopped"
fi
if [[ "$STATE" == "stopped" ]]; then
  echo "==> Starting"
  aws ec2 start-instances --instance-ids "$INSTANCE_ID" >/dev/null
fi

echo "==> Waiting for running state"
aws ec2 wait instance-running --instance-ids "$INSTANCE_ID"

INSTANCE_IP=$(aws ec2 describe-instances --instance-ids "$INSTANCE_ID" \
  --query 'Reservations[0].Instances[0].PublicIpAddress' --output text)
if [[ -z "$INSTANCE_IP" || "$INSTANCE_IP" == "None" ]]; then
  echo "no public IP on instance, refusing to update DuckDNS"
  exit 1
fi
echo "==> Public IP: $INSTANCE_IP"

echo "==> Updating DuckDNS"
RESP=$(curl -fsS "https://www.duckdns.org/update?domains=${DOMAIN}&token=${DUCKDNS_TOKEN}&ip=${INSTANCE_IP}")
[[ "$RESP" == "OK" ]] || {
  echo "DuckDNS update failed: $RESP"
  exit 1
}

echo "==> Waiting for headscale to be healthy"
tries=0
until curl -fsS --max-time 3 --resolve "${FQDN}:443:${INSTANCE_IP}" \
  "https://${FQDN}/health" >/dev/null 2>&1; do
  ((++tries > 30)) && {
    echo "headscale not healthy after ~2.5 min"
    exit 1
  }
  sleep 5
done
echo "    healthy"

echo "==> Flushing local DNS cache"
sudo resolvectl flush-caches 2>/dev/null || true

echo "===> Restarting local tailscale service to catch up"
sudo systemctl restart tailscaled
sleep 3
tailscale status

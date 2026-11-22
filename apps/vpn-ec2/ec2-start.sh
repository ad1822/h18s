#!/bin/sh
set -eu
. /scripts/common.sh

DOMAIN="h18s"
FQDN="${DOMAIN}.duckdns.org"

if [ "$STATE" = "stopping" ]; then
  echo "==> Waiting for it to finish stopping"
  aws ec2 wait instance-stopped --instance-ids "$INSTANCE_ID"
  STATE="stopped"
fi
if [ "$STATE" = "stopped" ]; then
  echo "==> Starting"
  aws ec2 start-instances --instance-ids "$INSTANCE_ID" >/dev/null
fi

echo "==> Waiting for running state"
aws ec2 wait instance-running --instance-ids "$INSTANCE_ID"

INSTANCE_IP=$(aws ec2 describe-instances --instance-ids "$INSTANCE_ID" \
  --query 'Reservations[0].Instances[0].PublicIpAddress' --output text)
if [ -z "$INSTANCE_IP" ] || [ "$INSTANCE_IP" = "None" ]; then
  echo "no public IP on instance, refusing to update DuckDNS"
  exit 1
fi
echo "==> Public IP: $INSTANCE_IP"

echo "==> Updating DuckDNS"
RESP=$(curl -fsS "https://www.duckdns.org/update?domains=${DOMAIN}&token=${DUCKDNS_TOKEN}&ip=${INSTANCE_IP}")
[ "$RESP" = "OK" ] || { echo "DuckDNS update failed: $RESP"; exit 1; }

echo "==> Waiting for headscale to be healthy"
tries=0
until curl -fsS --max-time 3 --resolve "${FQDN}:443:${INSTANCE_IP}" \
  "https://${FQDN}/health" >/dev/null 2>&1; do
  tries=$((tries + 1))
  [ "$tries" -gt 30 ] && { echo "headscale not healthy after ~2.5 min"; exit 1; }
  sleep 5
done
echo "    healthy"
echo "NOTE: on the laptop run: sudo resolvectl flush-caches && sudo systemctl restart tailscaled"

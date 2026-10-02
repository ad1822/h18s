#!/bin/sh
# rclone sync every path in sources.txt (relative to /home/ad) to $BACKUP_DEST/<same path>
set -u

# rclone rewrites the config when it refreshes the gdrive token; the secret mount is read-only
cp /secret/rclone.conf /tmp/rclone.conf

failed=""
while read -r path; do
  case "$path" in "" | \#*) continue ;; esac
  echo "==> $path"
  rclone --config /tmp/rclone.conf \
    sync "/home/ad/$path" "$BACKUP_DEST/$path" \
    --fast-list \
    --transfers 8 \
    --checkers 16 \
    --drive-chunk-size 64M \
    --buffer-size 64M \
    --multi-thread-streams 4 \
    --exclude ".git/**" \
    --exclude "node_modules/**" \
    --exclude ".cache/**" \
    --stats-one-line \
    --stats 1m \
    || failed="$failed $path"
done < /scripts/sources.txt

if [ -n "$failed" ]; then
  echo "FAILED:$failed"
  exit 1
fi
echo "all backups done"

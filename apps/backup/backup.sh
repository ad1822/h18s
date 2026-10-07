#!/bin/sh
# rclone sync every path in sources.txt (relative to /home/ad) to $BACKUP_DEST/<same path>
# files deleted or overwritten locally are moved to $ARCHIVE_DEST/<YYYY-MM-DD>/<same path>, kept $ARCHIVE_KEEP_DAYS days
set -u

# rclone rewrites the config when it refreshes the gdrive token; the secret mount is read-only
cp /secret/rclone.conf /tmp/rclone.conf

today=$(date +%F)

failed=""
while read -r path; do
  case "$path" in "" | \#*) continue ;; esac
  echo "==> $path"
  rclone --config /tmp/rclone.conf \
    sync "/home/ad/$path" "$BACKUP_DEST/$path" \
    --backup-dir "$ARCHIVE_DEST/$today/$path" \
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

# purge archive folders by their date name, not file mtime, so each deletion is kept the full period
cutoff=$(date -d "@$(( $(date +%s) - ARCHIVE_KEEP_DAYS * 86400 ))" +%Y%m%d)
echo "==> purging $ARCHIVE_DEST folders older than $ARCHIVE_KEEP_DAYS days"
for dir in $(rclone --config /tmp/rclone.conf lsf --dirs-only "$ARCHIVE_DEST" 2>/dev/null); do
  day=$(echo "$dir" | tr -d '/-')
  case "$day" in *[!0-9]* | "") continue ;; esac # skip anything that isn't a YYYY-MM-DD folder
  if [ "$day" -lt "$cutoff" ]; then
    echo "purge $dir"
    rclone --config /tmp/rclone.conf purge "$ARCHIVE_DEST/$dir" || failed="$failed archive:$dir"
  fi
done

if [ -n "$failed" ]; then
  echo "FAILED:$failed"
  exit 1
fi
echo "all backups done"

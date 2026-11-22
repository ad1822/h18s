# sourced by ec2-start.sh / ec2-stop.sh

# sealed values often carry a trailing newline, which breaks the signed AWS headers
AWS_ACCESS_KEY_ID=$(printf %s "$AWS_ACCESS_KEY_ID" | tr -d '\r\n')
AWS_SECRET_ACCESS_KEY=$(printf %s "$AWS_SECRET_ACCESS_KEY" | tr -d '\r\n')
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY

# The HeadScale instance. The role is only allowed to act on this one.
INSTANCE_ID="i-0a0ce0a07cce24fb0"
case "$INSTANCE_ID" in i-REPLACE_ME) echo "set INSTANCE_ID in common.sh"; exit 1 ;; esac

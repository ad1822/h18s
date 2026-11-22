# sealed values often carry a trailing newline, which breaks the signed AWS headers
AWS_ACCESS_KEY_ID=$(printf %s "$AWS_ACCESS_KEY_ID" | tr -d '\r\n')
AWS_SECRET_ACCESS_KEY=$(printf %s "$AWS_SECRET_ACCESS_KEY" | tr -d '\r\n')
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY

# sourced by ec2-start.sh / ec2-stop.sh
NAME_TAG="HeadScale"

echo "==> Finding instance"
INSTANCE_ID=$(aws ec2 describe-instances \
  --filters "Name=tag:Name,Values=${NAME_TAG}" \
  "Name=instance-state-name,Values=pending,running,stopping,stopped" \
  --query 'Reservations[0].Instances[0].InstanceId' --output text)
if [ -z "$INSTANCE_ID" ] || [ "$INSTANCE_ID" = "None" ]; then
  echo "instance with Name=${NAME_TAG} not found"
  exit 1
fi
echo "    $INSTANCE_ID"

STATE=$(aws ec2 describe-instances --instance-ids "$INSTANCE_ID" \
  --query 'Reservations[0].Instances[0].State.Name' --output text)
echo "==> Current state: $STATE"

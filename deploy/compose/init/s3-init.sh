#!/bin/sh
# Creates the product-images bucket (spec 7.4) and makes thumbs/ publicly
# readable; originals/ stays private. Idempotent.
# No lifecycle rule: the catalog worker removes unconfirmed uploads (R8, R21).
set -eu

B=product-images
aws() { command aws --endpoint-url "$S3_ENDPOINT" "$@"; }

aws s3api head-bucket --bucket "$B" 2>/dev/null || aws s3api create-bucket --bucket "$B"

cat > /tmp/policy.json <<JSON
{
  "Version": "2012-10-17",
  "Statement": [{
    "Sid": "PublicThumbnails",
    "Effect": "Allow",
    "Principal": "*",
    "Action": ["s3:GetObject"],
    "Resource": ["arn:aws:s3:::$B/thumbs/*"]
  }]
}
JSON
aws s3api put-bucket-policy --bucket "$B" --policy file:///tmp/policy.json
echo "bucket $B ready"

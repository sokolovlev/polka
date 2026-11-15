#!/bin/sh
# Writes the S3 identity file from environment variables (secrets stay out of
# the repo) and starts SeaweedFS: master + volume + filer + S3 API in one
# process, which is enough for local development.
set -eu

cat > /tmp/s3.json <<JSON
{
  "identities": [{
    "name": "polka",
    "credentials": [{ "accessKey": "$S3_ACCESS_KEY", "secretKey": "$S3_SECRET_KEY" }],
    "actions": ["Admin", "Read", "Write", "List", "Tagging"]
  }]
}
JSON
exec weed server -dir=/data -ip.bind=0.0.0.0 -s3 -s3.port=8333 -s3.config=/tmp/s3.json

"""SeaweedFS S3 spike for ADR-0003 (R7): presigned PUT with signed headers,
lifecycle on originals/, anonymous read on thumbs/.

Run against `make infra` (credentials from .env):
    pip install boto3 requests
    python docs/adr/assets/0003-s3-spike.py http://localhost:8333

The lifecycle check only proves the rule is stored, not enforced: see ADR-0003.
"""
import json, os, sys, time
import boto3, requests
from botocore.config import Config

EP = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:18333"
s3 = boto3.client("s3", endpoint_url=EP, aws_access_key_id=os.environ.get("S3_ACCESS_KEY", "polka"),
                  aws_secret_access_key=os.environ.get("S3_SECRET_KEY", "polka-local-secret"),
                  region_name="us-east-1", config=Config(signature_version="s3v4", s3={"addressing_style": "path"}))
B = "product-images"
res = {}
def check(name, ok, detail=""):
    res[name] = ok
    print(f"{'PASS' if ok else 'FAIL'} {name} {detail}")

try:
    s3.create_bucket(Bucket=B)
except s3.exceptions.BucketAlreadyOwnedByYou:
    pass

# 1. presigned PUT with Content-Type and Content-Length in the signature
body = b"x" * 100
url = s3.generate_presigned_url("put_object", ExpiresIn=600,
    Params={"Bucket": B, "Key": "originals/p1/img1", "ContentType": "image/webp", "ContentLength": len(body)})
signed = url.split("X-Amz-SignedHeaders=")[1].split("&")[0]
check("presign.signed_headers_include_type_and_length", "content-type" in signed and "content-length" in signed, signed)
r = requests.put(url, data=body, headers={"Content-Type": "image/webp"})
check("presign.correct_upload_accepted", r.status_code == 200, r.status_code)
r = requests.put(url, data=body, headers={"Content-Type": "image/png"})
check("presign.wrong_content_type_rejected", r.status_code == 403, r.status_code)
r = requests.put(url, data=b"x" * 5000, headers={"Content-Type": "image/webp"})
check("presign.wrong_length_rejected", r.status_code == 403, r.status_code)
url_exp = s3.generate_presigned_url("put_object", ExpiresIn=1,
    Params={"Bucket": B, "Key": "originals/p1/img2", "ContentType": "image/webp", "ContentLength": 100})
time.sleep(2)
r = requests.put(url_exp, data=body, headers={"Content-Type": "image/webp"})
check("presign.expired_rejected", r.status_code == 403, r.status_code)

# 2. lifecycle expiration for originals/
try:
    s3.put_bucket_lifecycle_configuration(Bucket=B, LifecycleConfiguration={"Rules": [{
        "ID": "expire-unconfirmed", "Status": "Enabled", "Filter": {"Prefix": "originals/"},
        "Expiration": {"Days": 1}}]})
    got = s3.get_bucket_lifecycle_configuration(Bucket=B)["Rules"]
    check("lifecycle.put_and_get", got[0]["Filter"].get("Prefix") == "originals/" and got[0]["Expiration"]["Days"] == 1, got)
except Exception as e:
    check("lifecycle.put_and_get", False, repr(e)[:200])

# 3. anonymous read for thumbs/ only
s3.put_object(Bucket=B, Key="thumbs/200/img1.webp", Body=b"thumb", ContentType="image/webp")
policy = {"Version": "2012-10-17", "Statement": [{"Sid": "PublicThumbs", "Effect": "Allow", "Principal": "*",
          "Action": ["s3:GetObject"], "Resource": [f"arn:aws:s3:::{B}/thumbs/*"]}]}
try:
    s3.put_bucket_policy(Bucket=B, Policy=json.dumps(policy))
    check("policy.put", True)
except Exception as e:
    check("policy.put", False, repr(e)[:200])
r = requests.get(f"{EP}/{B}/thumbs/200/img1.webp")
check("anon.thumbs_readable", r.status_code == 200 and r.content == b"thumb", r.status_code)
r = requests.get(f"{EP}/{B}/originals/p1/img1")
check("anon.originals_private", r.status_code == 403, r.status_code)
r = requests.put(f"{EP}/{B}/thumbs/200/evil.webp", data=b"x")
check("anon.thumbs_not_writable", r.status_code == 403, r.status_code)
r = requests.get(f"{EP}/{B}?list-type=2")
check("anon.no_listing", r.status_code == 403, r.status_code)
print(json.dumps(res))

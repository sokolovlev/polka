# 0003. S3 storage: SeaweedFS instead of MinIO

- Status: Accepted
- Date: 2026-11-14
- Spec: 1, 4.4, 7.4, 9.2, R6, R7, R8

## Context

The spec named MinIO for product photos. In October 2025 MinIO stopped
publishing community images and binaries, and the community edition is no
longer maintained: `docker pull minio/minio` does not give a current build. We
need a free (R6) S3-compatible store that supports what 7.4 relies on:

1. presigned PUT with `Content-Type` and `Content-Length` fixed in the signature;
2. lifecycle expiry of unconfirmed uploads after 24 h;
3. anonymous read for `thumbs/`, private `originals/`.

Catalog talks only to the S3 API (AWS SDK), so the backend is swappable.

## Options

1. **SeaweedFS** (Apache 2.0), `weed server -s3`. One process for dev,
   actively maintained.
2. **Garage** (AGPL). Light, but anonymous access is done through its
   "website" mode rather than bucket policies.
3. **A community-built MinIO image.** Upstream is unmaintained, so security
   fixes depend on a third party. Weakest answer in an interview.

## Decision

SeaweedFS 4.51, after a spike against the real server
([`assets/0003-s3-spike.py`](assets/0003-s3-spike.py), run against
`make infra`):

| Check | Result |
|---|---|
| Presigned PUT signs `content-length;content-type;host` | PASS |
| Upload with the signed type and length | 200 |
| Wrong `Content-Type` | 403 |
| Wrong length (5000 bytes for a 100-byte signature) | 403 |
| Expired URL | 403 |
| Bucket policy: `s3:GetObject` on `thumbs/*` for `*` | anonymous GET 200 |
| Anonymous GET of `originals/…` | 403 |
| Anonymous PUT and anonymous listing | 403 |
| Lifecycle rule (prefix `originals/`, 1 day) stored and returned | PASS, **but see below** |

**Lifecycle is stored but not enforced by default.** Objects written after the
rule have no TTL (`TtlSec=0` in filer metadata). SeaweedFS enforces lifecycle
in two ways:

- *Fast path* (`s3.bucket.lifecycle.fastpath -enable`): `Expiration.Days` is
  stamped as a volume TTL at write time. Verified: a new object under
  `originals/` got `TtlSec=86400`, one under `thumbs/` got 0. The TTL is baked
  into the object and cannot be removed later.
- *Worker-driven* expiry: a separate admin + worker plugin stack. Not tried.

This exposes a gap in the spec itself, independent of the backend: a lifecycle
rule on prefix `originals/` deletes **every** original after 24 h, confirmed
or not. "Delete only unconfirmed uploads" needs either a tag filter
(confirmation removes the tag, which needs worker-driven expiry) or a separate
prefix for uploads that confirmation moves out of (question Q10).

Resolution (R21): no lifecycle rule. `s3-init` creates the bucket and the
`thumbs/` policy only; unconfirmed uploads are removed by the catalog worker
(R8). For a local prototype this is one mechanism instead of two, and it
works the same on any S3 backend.

Image source: `chrislusf/seaweedfs` on Docker Hub. Compose runs it as a single
`weed server -s3` process with the S3 identity generated from `.env` at start.
Kubernetes (stage 5) runs it as a plain StatefulSet (R11).

## Consequences

- No vendor-specific code: catalog uses the AWS SDK for Go v2 with path-style
  addressing.
- The spike script is kept so the check can be repeated after a SeaweedFS
  upgrade.
- If a second line of defence is ever needed, upload to a separate
  `uploads/` prefix with a 24 h rule (the fast path enforces it) and copy to
  `originals/` on confirmation.

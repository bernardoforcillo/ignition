# go-packages/storage

Object storage behind short-lived signed URLs: the browser uploads and downloads **straight to the
bucket**, and the Go service only signs, verifies and deletes. Stdlib only, no environment, no
transport.

```
Signer / Store (port) <-- S3 (AWS, R2, MinIO, GCS interop) | GCS (V4) | storagetest.Server (tests)
```

## Use

```go
store, err := storage.NewS3(storage.S3Config{Bucket: "b", Region: "auto", AccessKeyID: id,
    SecretAccessKey: secret, Endpoint: "https://<acct>.r2.cloudflarestorage.com"})
// or: storage.NewGCS(storage.GCSConfig{Bucket: "b", CredentialsJSON: serviceAccountKeyJSON})

put, _ := store.PresignPut(ctx, "workspaces/<id>/<uuid>", storage.PutOptions{
    ContentType: "application/pdf", Size: 1234, Expires: 10 * time.Minute})
// hand put.URL, put.Method, put.Headers and put.ExpiresAt to the client

obj, err := store.Stat(ctx, key)   // storage.ErrNotFound when the client never uploaded
get, _ := store.PresignGet(ctx, key, storage.GetOptions{Filename: "report.pdf"}) // attachment
err = store.Delete(ctx, key)       // deleting a missing object succeeds
```

Consumers declare the slice they need (`Signer`, or `Store` when they also verify and delete).

## What a signed upload enforces

| | S3-compatible | GCS |
|---|---|---|
| Content type | signed `content-type` header | signed `content-type` header |
| Exact size | signed `content-length` header (the browser sets it from the body) | signed `x-goog-content-length-range: N,N`, listed in `Signed.Headers`, which the client must send |
| Lifetime | `X-Amz-Expires` (default 10 min, max 1 h here) | `X-Goog-Expires`, same bounds |
| Payload | `UNSIGNED-PAYLOAD` (streamed) | `UNSIGNED-PAYLOAD` |

A request that differs from what was signed is refused by the storage service. The service still
verifies with `Stat` that the object that arrived is the one it announced.

## Why no vendor SDK

`cloud.google.com/go/storage` and `aws-sdk-go-v2` pull dozens of transitive modules (auth,
telemetry, gRPC, protobuf, retry) into the gateway's supply chain to do what is, here, a page of
HMAC or RSA signing. `Stat` and `Delete` are HEAD and DELETE requests through URLs the adapter
signs for itself, so there is no OAuth token exchange either. Signing is checked against AWS's
published SigV4 example and against `storagetest.Server`, whose verifier is written independently
of the adapters. Trade-off: GCS needs a service-account **key** (a secret); where keys are
undesirable (Workload Identity) use the S3 adapter against `https://storage.googleapis.com` with
an HMAC key for a service account.

## Bucket setup (CORS)

The browser PUTs and GETs cross-origin, so the bucket needs a CORS rule for the web origin:
methods `PUT, GET, HEAD`, request headers `Content-Type` and (GCS) `x-goog-content-length-range`,
exposed header `ETag`, a short `MaxAgeSeconds`. The bucket stays private; there is no public ACL.

## Tests

`storagetest.New(t)` starts an in-memory server that verifies SigV4 and V4 signatures from the
request as it arrives (host, signed headers, query, expiry, the signed length range) and answers
CORS preflights. `srv.S3(t)` and `srv.GCS(t)` return adapters pointed at it; `srv.SetNow` moves the
clock for expiry tests. The gateway's end-to-end harness runs the same idea in TypeScript.

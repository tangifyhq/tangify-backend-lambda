# api-proxy-cf-worker

Cloudflare Worker that reverse-proxies `api.tangify.in` to the Tangify Lambda Function URL and rewrites the `Host` header.

Lambda Function URLs require `Host` to be the `*.lambda-url.*.on.aws` hostname. Proxied Cloudflare DNS that keeps `Host: api.tangify.in` fails (often Cloudflare **522**).

## Origin

Set in `wrangler.toml` `[vars]`:

```toml
LAMBDA_ORIGIN = "https://vrhftl7y7adnf6z3f7qlldgyvy0cqniv.lambda-url.ap-south-1.on.aws"
```

Update this if the Function URL changes (SAM stack output `TangifyBackendLambdaUrl`).

## Deploy

```bash
cd api-proxy-cf-worker
./deploy.sh
```

Attach `api.tangify.in` manually in Cloudflare (Workers → Domains & Routes, or DNS + Worker Route). Do not put `[[routes]]` in `wrangler.toml` unless you want deploy to manage it.

Keep the DNS record **proxied** (orange cloud).

## Smoke

```bash
curl -sS https://api.tangify.in/api/v1/health
# {"status":"ok"}

curl -sS -X POST https://api.tangify.in/api/v1/web/auth/continue \
  -H 'Content-Type: application/json' \
  -d '{"key":"ZZZZZZZ"}'
# {"error":"login link expired or already used"}
```

Direct Function URL (no worker) should also work:

```bash
curl -sS https://vrhftl7y7adnf6z3f7qlldgyvy0cqniv.lambda-url.ap-south-1.on.aws/api/v1/health
```

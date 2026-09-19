/**
 * Reverse proxy for api.tangify.in → Lambda Function URL.
 *
 * Lambda Function URLs reject requests whose Host is not the *.lambda-url.*.on.aws
 * hostname. Cloudflare orange-cloud / DNS origins send Host: api.tangify.in, which
 * causes connection failures (often surfaced as Cloudflare 522).
 *
 * This worker rewrites the origin URL and Host header to the Function URL host,
 * and adds CORS for the ordering web app.
 */

interface Env {
  /** e.g. https://xxxx.lambda-url.ap-south-1.on.aws */
  LAMBDA_ORIGIN: string;
}

const ALLOWED_ORIGINS = new Set([
  "https://order.tangify.in",
  "https://tangify-ordering-web.vercel.app",
  "http://localhost:5173",
  "http://127.0.0.1:5173",
]);

function corsHeaders(request: Request): Headers {
  const origin = request.headers.get("Origin") || "";
  const headers = new Headers();
  if (ALLOWED_ORIGINS.has(origin)) {
    headers.set("Access-Control-Allow-Origin", origin);
    headers.set("Vary", "Origin");
  }
  headers.set(
    "Access-Control-Allow-Methods",
    "GET,POST,PUT,PATCH,DELETE,OPTIONS",
  );
  headers.set(
    "Access-Control-Allow-Headers",
    "Content-Type, Authorization, X-Tangify-Environment, X-Bootstrap-Secret, X-CF-Secret",
  );
  headers.set("Access-Control-Max-Age", "86400");
  return headers;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const cors = corsHeaders(request);
    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: cors });
    }

    const originRaw = (env.LAMBDA_ORIGIN || "").trim().replace(/\/+$/, "");
    if (!originRaw) {
      return json({ error: "LAMBDA_ORIGIN is not configured" }, 500, cors);
    }

    let origin: URL;
    try {
      origin = new URL(originRaw);
    } catch {
      return json({ error: "LAMBDA_ORIGIN is invalid" }, 500, cors);
    }

    const incoming = new URL(request.url);
    const target = new URL(incoming.pathname + incoming.search, origin);

    const headers = new Headers(request.headers);
    headers.set("Host", origin.host);
    headers.delete("X-Forwarded-Host");

    const init: RequestInit = {
      method: request.method,
      headers,
      redirect: "manual",
    };
    if (request.method !== "GET" && request.method !== "HEAD") {
      init.body = request.body;
    }

    try {
      const upstream = await fetch(target.toString(), init);
      const out = new Headers(upstream.headers);
      cors.forEach((value, key) => out.set(key, value));
      return new Response(upstream.body, {
        status: upstream.status,
        statusText: upstream.statusText,
        headers: out,
      });
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "upstream fetch failed";
      return json({ error: message }, 502, cors);
    }
  },
};

function json(
  payload: unknown,
  status = 200,
  extra?: Headers,
): Response {
  const headers = new Headers(extra);
  headers.set("content-type", "application/json");
  return new Response(JSON.stringify(payload), { status, headers });
}

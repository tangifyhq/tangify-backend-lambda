import { DurableObject } from "cloudflare:workers";

interface Env {
  WEB_ORDER_COUNTER: DurableObjectNamespace<WebOrderYearCounter>;
}

type OrderRequest = {
  attempt_id: string;
};

type OrderResponse = {
  order_ref: string;
  attempt_id: string;
  year: number;
  sequence: number;
};

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    if (request.method !== "POST") {
      return json({ error: "method not allowed" }, 405);
    }

    let body: OrderRequest;
    try {
      body = (await request.json()) as OrderRequest;
    } catch {
      return json({ error: "invalid JSON body" }, 400);
    }

    const attemptID = body.attempt_id?.trim();
    if (!attemptID) {
      return json({ error: "attempt_id is required" }, 400);
    }

    const year = new Date().getUTCFullYear();

    // idFromName deterministically maps the year to one DO instance.
    const durableID = env.WEB_ORDER_COUNTER.idFromName(String(year));
    const stub = env.WEB_ORDER_COUNTER.get(durableID);

    const doResp = await stub.fetch("https://do.internal/web-order/next", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ attempt_id: attemptID, year }),
    });

    const result = (await doResp.json()) as OrderResponse | { error: string };
    return json(result, doResp.status);
  },
};

export class WebOrderYearCounter extends DurableObject<Env> {
  private readonly state: DurableObjectState;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    this.state = ctx;
  }

  async fetch(request: Request): Promise<Response> {
    if (request.method !== "POST") {
      return json({ error: "method not allowed" }, 405);
    }

    let body: { attempt_id?: string; year?: number };
    try {
      body = (await request.json()) as { attempt_id?: string; year?: number };
    } catch {
      return json({ error: "invalid JSON body" }, 400);
    }

    const attemptID = body.attempt_id?.trim();
    const year = body.year;
    if (!attemptID || !year) {
      return json({ error: "attempt_id and year are required" }, 400);
    }

    try {
      const result = await this.state.storage.transaction(async (txn) => {
        const byAttemptKey = `attempt:${attemptID}`;
        const existing = await txn.get<OrderResponse>(byAttemptKey);
        if (existing) {
          return existing;
        }

        const current = (await txn.get<number>("counter")) ?? 0;
        const sequence = current + 1;
        const orderRef = `TGFY-W-${year}-${String(sequence).padStart(6, "0")}`;
        const byOrderKey = `ord:${orderRef}`;

        const payload: OrderResponse = {
          order_ref: orderRef,
          attempt_id: attemptID,
          year,
          sequence,
        };

        await txn.put("counter", sequence);
        await txn.put(byAttemptKey, payload);
        await txn.put(byOrderKey, payload);

        return payload;
      });

      return json(result, 200);
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "failed to generate order number";
      return json({ error: message }, 500);
    }
  }
}

function json(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "content-type": "application/json" },
  });
}

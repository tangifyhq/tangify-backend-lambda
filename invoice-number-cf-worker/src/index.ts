import { DurableObject } from "cloudflare:workers";

interface Env {
  INVOICE_COUNTER: DurableObjectNamespace<InvoiceYearCounter>;
}

type InvoiceRequest = {
  bill_id: string;
};

type InvoiceResponse = {
  invoice_number: string;
  bill_id: string;
  year: number;
  sequence: number;
};

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    if (request.method !== "POST") {
      return json({ error: "method not allowed" }, 405);
    }

    let body: InvoiceRequest;
    try {
      body = (await request.json()) as InvoiceRequest;
    } catch {
      return json({ error: "invalid JSON body" }, 400);
    }

    const billID = body.bill_id?.trim();
    if (!billID) {
      return json({ error: "bill_id is required" }, 400);
    }

    const year = new Date().getUTCFullYear();

    // idFromName deterministically maps the year to one DO instance.
    // The instance is created on first request if it doesn't exist yet.
    const durableID = env.INVOICE_COUNTER.idFromName(String(year));
    const stub = env.INVOICE_COUNTER.get(durableID);

    const doResp = await stub.fetch("https://do.internal/invoice/next", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ bill_id: billID, year }),
    });

    const result = (await doResp.json()) as InvoiceResponse | { error: string };
    return json(result, doResp.status);
  },
};

export class InvoiceYearCounter extends DurableObject<Env> {
  private readonly state: DurableObjectState;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    this.state = ctx;
  }

  async fetch(request: Request): Promise<Response> {
    if (request.method !== "POST") {
      return json({ error: "method not allowed" }, 405);
    }

    let body: { bill_id?: string; year?: number };
    try {
      body = (await request.json()) as { bill_id?: string; year?: number };
    } catch {
      return json({ error: "invalid JSON body" }, 400);
    }

    const billID = body.bill_id?.trim();
    const year = body.year;
    if (!billID || !year) {
      return json({ error: "bill_id and year are required" }, 400);
    }

    try {
      const result = await this.state.storage.transaction(async (txn) => {
        const byBillKey = `bill:${billID}`;
        const existing = await txn.get<InvoiceResponse>(byBillKey);
        if (existing) {
          return existing;
        }

        const current = (await txn.get<number>("counter")) ?? 0;
        const sequence = current + 1;
        const invoiceNumber = `${year}-${String(sequence).padStart(6, "0")}`;
        const byInvoiceKey = `inv:${invoiceNumber}`;

        const payload: InvoiceResponse = {
          invoice_number: invoiceNumber,
          bill_id: billID,
          year,
          sequence,
        };

        // Store in two maps in one atomic storage transaction.
        await txn.put("counter", sequence);
        await txn.put(byBillKey, payload);
        await txn.put(byInvoiceKey, payload);

        return payload;
      });

      return json(result, 200);
    } catch (err) {
      const message = err instanceof Error ? err.message : "failed to generate invoice number";
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

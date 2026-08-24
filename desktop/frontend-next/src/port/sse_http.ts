import { HttpError } from "./port";

type Refusal = { code?: string; error?: string; params?: Record<string, string | number> };

// The transport half of SsePort: where the kernel is, and the four shapes every
// call to it takes. Split out because the port itself is the whole AgentPort
// surface — a hundred endpoint methods — and none of them should have to be
// read past to find how a request is actually made.
export class SseHttp {
  // rt names the pane this port speaks for. The shell's bus carries every
  // pane's frames, so a channel per runtime is what keeps two live
  // conversations out of each other's transcript.
  constructor(
    protected readonly base = "",
    protected readonly rt = "",
  ) {}

  // A refusal carries a code; only when it does not do we fall back to text.
  // Throwing HttpError with the reason attached keeps that choice at the point
  // that renders it, instead of flattening it to a string here.
  protected static async fail(path: string, res: Response): Promise<never> {
    // Read once as text: a refusal envelope is JSON, but http.Error writes the
    // reason as plain text, and parsing first threw that account away and left
    // the panel showing only a path and a number.
    const raw = (await res.text().catch(() => "")).trim();
    let body: Refusal | null = null;
    try {
      const parsed: unknown = raw === "" ? null : JSON.parse(raw);
      if (parsed && typeof parsed === "object") body = parsed as Refusal;
    } catch {
      // Plain text, which is the whole message.
    }
    const detail = body?.error || raw.slice(0, 400);
    throw new HttpError(res.status, detail || `${path}: ${res.status}`, body ?? undefined, detail !== "");
  }

  protected async post(path: string, body?: unknown): Promise<void> {
    const res = await fetch(this.base + path, {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) await SseHttp.fail(path, res);
  }

  // A POST whose answer is the payload, not a status code.
  protected async post0<T>(path: string, body?: unknown): Promise<T> {
    const res = await fetch(this.base + path, {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) await SseHttp.fail(path, res);
    return (await res.json()) as T;
  }

  protected async del(path: string): Promise<void> {
    const res = await fetch(this.base + path, { method: "DELETE", credentials: "same-origin" });
    if (!res.ok) await SseHttp.fail(path, res);
  }

  protected async get<T>(path: string): Promise<T> {
    const res = await fetch(this.base + path, { credentials: "same-origin" });
    if (!res.ok) throw new Error(`${path}: ${res.status}`);
    return (await res.json()) as T;
  }
}

// A capability scope rides as a query rather than a path segment: every one of
// these endpoints answers for the running workspace when it is absent.
export function rootQuery(root?: string): string {
  return root ? "?root=" + encodeURIComponent(root) : "";
}

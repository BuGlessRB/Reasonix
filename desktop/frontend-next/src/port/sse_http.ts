import { HttpError } from "./port";

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
    const body = (await res.json().catch(() => null)) as
      | { code?: string; error?: string; params?: Record<string, string | number> }
      | null;
    throw new HttpError(res.status, body?.error || `${path}: ${res.status}`, body ?? undefined);
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

  protected async get<T>(path: string): Promise<T> {
    const res = await fetch(this.base + path, { credentials: "same-origin" });
    if (!res.ok) throw new Error(`${path}: ${res.status}`);
    return (await res.json()) as T;
  }
}

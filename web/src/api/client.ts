export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(message: string, status: number, code = "") {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

interface ErrorPayload {
  error?: unknown;
  code?: unknown;
}

async function parseBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function errorFromResponse(response: Response, data: unknown): ApiError {
  const payload = data && typeof data === "object" ? (data as ErrorPayload) : {};
  const message = payload.error ? String(payload.error) : `请求失败（HTTP ${response.status}）`;
  const code = payload.code ? String(payload.code) : "";
  return new ApiError(message, response.status, code);
}

function redirectToLogin(): void {
  if (typeof window !== "undefined") {
    window.location.href = "/login";
  }
}

export async function requestJSON<T>(
  path: string,
  options: RequestInit = {},
  redirectOn401 = true,
): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.body && !(options.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(path, { ...options, headers });
  if (redirectOn401 && response.status === 401) {
    redirectToLogin();
    throw new ApiError("登录已过期，正在跳转到登录页…", 401, "unauthorized");
  }
  const data = await parseBody(response);
  if (!response.ok) throw errorFromResponse(response, data);
  return data as T;
}

export function withQuery(path: string, params: Record<string, string> = {}): string {
  const query = new URLSearchParams(params).toString();
  return query ? `${path}?${query}` : path;
}

export async function downloadAttachment(
  path: string,
  params: Record<string, string>,
  filename: string,
): Promise<void> {
  const response = await fetch(withQuery(path, params));
  if (response.status === 401) {
    redirectToLogin();
    throw new ApiError("登录已过期，正在跳转到登录页…", 401, "unauthorized");
  }
  if (!response.ok) {
    throw errorFromResponse(response, await parseBody(response));
  }
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

export async function uploadForm<T>(path: string, form: FormData): Promise<T> {
  return requestJSON<T>(path, { method: "POST", body: form });
}

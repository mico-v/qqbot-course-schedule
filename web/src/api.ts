export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

export async function api<T = unknown>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  if (response.status === 401) {
    window.location.href = "/login";
    throw new ApiError("登录已过期，正在跳转到登录页…", 401);
  }
  const text = await response.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!response.ok) {
    const message =
      data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : `请求失败（HTTP ${response.status}）`;
    throw new ApiError(message, response.status);
  }
  return data as T;
}

export function apiGet<T>(path: string, params: Record<string, string> = {}): Promise<T> {
  const query = new URLSearchParams(params).toString();
  return api<T>(query ? `${path}?${query}` : path);
}

export function apiPost<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: "POST", body: JSON.stringify(body) });
}

// publicApi serves token-bearing endpoints without the admin 401 redirect.
export async function publicApi<T = unknown>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  const text = await response.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!response.ok) {
    const message =
      data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : `请求失败（HTTP ${response.status}）`;
    throw new ApiError(message, response.status);
  }
  return data as T;
}

export function publicApiGet<T>(
  path: string,
  params: Record<string, string> = {},
): Promise<T> {
  const query = new URLSearchParams(params).toString();
  return publicApi<T>(query ? `${path}?${query}` : path);
}

export function publicApiPost<T>(path: string, body: unknown): Promise<T> {
  return publicApi<T>(path, { method: "POST", body: JSON.stringify(body) });
}

// downloadFile fetches an attachment with the session cookie and triggers a
// browser download without navigating away from the SPA.
export async function downloadFile(
  path: string,
  params: Record<string, string>,
  filename: string,
): Promise<void> {
  const query = new URLSearchParams(params).toString();
  const response = await fetch(`${path}?${query}`);
  if (response.status === 401) {
    window.location.href = "/login";
    throw new ApiError("登录已过期，正在跳转到登录页…", 401);
  }
  if (!response.ok) {
    let message = `下载失败（HTTP ${response.status}）`;
    try {
      const data = await response.json();
      if (data && data.error) message = String(data.error);
    } catch {
      /* not JSON */
    }
    throw new ApiError(message, response.status);
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
  const response = await fetch(path, { method: "POST", body: form });
  if (response.status === 401) {
    window.location.href = "/login";
    throw new ApiError("登录已过期，正在跳转到登录页…", 401);
  }
  const text = await response.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!response.ok) {
    const message =
      data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : `请求失败（HTTP ${response.status}）`;
    throw new ApiError(message, response.status);
  }
  return data as T;
}

import type { PublicPageSchedule, PublicSaveResult } from "../types";
import { requestJSON, withQuery } from "./client";

export function fetchPublicSchedule(token: string): Promise<PublicPageSchedule> {
  return requestJSON<PublicPageSchedule>(
    withQuery("/api/public/schedule", { token }),
    {},
    false,
  );
}

export function savePublicSchedule(
  token: string,
  payload: unknown,
): Promise<PublicSaveResult> {
  return requestJSON<PublicSaveResult>(
    withQuery("/api/public/schedule/save", { token }),
    { method: "POST", body: JSON.stringify(payload) },
    false,
  );
}

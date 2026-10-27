import type {
  BotSettings,
  DayOverrideRow,
  ImportResult,
  PageSchedule,
  PendingMember,
  SavePageResult,
  Scope,
} from "../types";
import { downloadAttachment, requestJSON, uploadForm, withQuery } from "./client";

export async function fetchScopes(): Promise<Scope[]> {
  const data = await requestJSON<{ scopes?: Scope[] }>("/api/scopes");
  return data.scopes || [];
}

export function fetchSchedule(scopeId: string, userId: string): Promise<PageSchedule> {
  return requestJSON<PageSchedule>(withQuery("/api/schedule", { scope_id: scopeId, user_id: userId }));
}

export function saveSchedule(payload: unknown): Promise<SavePageResult> {
  return requestJSON<SavePageResult>("/api/schedule/save", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function fetchOverrides(scopeId: string): Promise<DayOverrideRow[]> {
  const data = await requestJSON<{ overrides?: DayOverrideRow[] }>(
    withQuery("/api/overrides", { scope_id: scopeId }),
  );
  return data.overrides || [];
}

export function saveOverride(payload: unknown): Promise<unknown> {
  return requestJSON("/api/overrides/set", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function removeOverride(payload: unknown): Promise<unknown> {
  return requestJSON("/api/overrides/delete", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function fetchPendingMembers(scopeId: string): Promise<{
  members: PendingMember[];
  note: string;
}> {
  const data = await requestJSON<{ members?: PendingMember[]; note?: string }>(
    withQuery("/api/members", { scope_id: scopeId }),
  );
  return { members: data.members || [], note: data.note || "" };
}

export function createSchedules(scopeId: string, members: PendingMember[]): Promise<{
  created_count: number;
}> {
  return requestJSON("/api/schedule/create", {
    method: "POST",
    body: JSON.stringify({ scope_id: scopeId, members }),
  });
}

export async function fetchSettings(): Promise<BotSettings> {
  const data = await requestJSON<{ settings?: BotSettings }>("/api/settings");
  return data.settings || ({} as BotSettings);
}

export function persistSettings(settings: BotSettings): Promise<unknown> {
  return requestJSON("/api/settings", {
    method: "POST",
    body: JSON.stringify(settings),
  });
}

export function downloadICS(scopeId: string, userId: string, filename: string): Promise<void> {
  return downloadAttachment(
    "/api/export",
    { scope_id: scopeId, user_id: userId, format: "ics" },
    filename,
  );
}

export function uploadAdminFile(form: FormData): Promise<ImportResult> {
  return uploadForm<ImportResult>("/api/import", form);
}

export function login(password: string): Promise<unknown> {
  return requestJSON("/api/login", {
    method: "POST",
    body: JSON.stringify({ password }),
  });
}

export function logout(): Promise<unknown> {
  return requestJSON("/api/logout", { method: "POST" });
}

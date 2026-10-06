import { computed, reactive } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiError, apiGet, apiPost, downloadFile } from "../api";
import type {
  BotSettings,
  CheckinBoard,
  CheckinRecord,
  CheckinTotal,
  DayOverrideRow,
  ImportResult,
  PageSchedule,
  PendingMember,
  SavePageResult,
  Scope,
} from "../types";
import { initialWeekStart, weekOccurrences } from "../utils/rrule";
import { startOfWeek } from "../utils/datetime";

export const state = reactive({
  scopes: [] as Scope[],
  selectedScopeId: "",
  selectedUserId: "",
  schedule: null as PageSchedule | null,
  overrides: [] as DayOverrideRow[],
  checkinTotals: [] as CheckinTotal[],
  checkinRecords: [] as CheckinRecord[],
  dirty: false,
  calendarWeekStart: startOfWeek(new Date()),
  search: "",
  loading: false,
});

export const currentScope = computed(
  () => state.scopes.find((scope) => scope.scope_id === state.selectedScopeId) || null,
);

export const selectedMemberLabel = computed(() => {
  const scope = currentScope.value;
  if (!scope || !state.selectedUserId) return "";
  const member = scope.members?.find((item) => item.user_id === state.selectedUserId);
  return member ? `${member.name || member.user_id}（${member.user_id}）` : "";
});

export function scopeMatches(scope: Scope, query: string): boolean {
  if (!query) return true;
  const haystack = [
    scope.label,
    scope.scope_id,
    scope.target_id,
    ...(scope.members || []).flatMap((member) => [member.name, member.user_id]),
  ]
    .join(" ")
    .toLocaleLowerCase();
  return haystack.includes(query.toLocaleLowerCase());
}

export function setDirty(value: boolean): void {
  state.dirty = value;
}

export async function canLeaveEditor(): Promise<boolean> {
  if (!state.dirty) return true;
  try {
    await ElMessageBox.confirm("当前课表有未保存修改，确定要放弃吗？", "未保存修改", {
      type: "warning",
      confirmButtonText: "放弃修改",
      cancelButtonText: "继续编辑",
    });
    return true;
  } catch {
    return false;
  }
}

export async function canExport(): Promise<boolean> {
  if (!state.dirty) return true;
  try {
    await ElMessageBox.confirm("当前课表有未保存的修改，导出的是已保存的版本。确定继续吗？", "导出提示", {
      type: "warning",
      confirmButtonText: "继续导出",
      cancelButtonText: "取消",
    });
    return true;
  } catch {
    return false;
  }
}

export async function loadScopes(keepSelection = true): Promise<void> {
  const data = await apiGet<{ scopes: Scope[] }>("/api/scopes");
  state.scopes = data.scopes || [];
  if (!keepSelection) {
    state.selectedScopeId = "";
    state.selectedUserId = "";
    state.schedule = null;
  }
}

export function selectScope(scope: Scope): void {
  state.selectedScopeId = scope.scope_id;
  state.selectedUserId = "";
  state.schedule = null;
  state.overrides = [];
  state.checkinTotals = [];
  state.checkinRecords = [];
  state.dirty = false;
}

let memberRequestId = 0;

export async function selectMember(scopeId: string, userId: string): Promise<void> {
  if (!(await canLeaveEditor())) return;
  const requestId = ++memberRequestId;
  try {
    const schedule = await apiGet<PageSchedule>("/api/schedule", {
      scope_id: scopeId,
      user_id: userId,
    });
    if (requestId !== memberRequestId) return;
    state.schedule = schedule;
    state.selectedScopeId = scopeId;
    state.selectedUserId = userId;
    state.calendarWeekStart = initialWeekStart(schedule.events || []);
    state.dirty = false;
    await Promise.all([loadOverrides(scopeId), loadCheckins(scopeId)]);
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

export async function reloadSelectedMember(): Promise<void> {
  if (!state.selectedScopeId || !state.selectedUserId) return;
  const schedule = await apiGet<PageSchedule>("/api/schedule", {
    scope_id: state.selectedScopeId,
    user_id: state.selectedUserId,
  });
  state.schedule = schedule;
  state.dirty = false;
  state.calendarWeekStart = initialWeekStart(schedule.events || []);
}

export async function refresh(): Promise<void> {
  if (!(await canLeaveEditor())) return;
  state.loading = true;
  try {
    await loadScopes();
    if (state.selectedScopeId && state.selectedUserId) {
      await reloadSelectedMember();
      await Promise.all([loadOverrides(state.selectedScopeId), loadCheckins(state.selectedScopeId)]);
    }
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    state.loading = false;
  }
}

export async function saveSchedule(): Promise<void> {
  const schedule = state.schedule;
  if (!schedule) return;
  const events = (schedule.events || []).map((event) => ({
    id: event.id || 0,
    uid: event.uid || "",
    course: String(event.course || "").trim(),
    start: String(event.start || "").trim(),
    end: String(event.end || "").trim(),
    location: String(event.location || "").trim(),
    rrule: String(event.rrule || "").trim(),
    description: String(event.description || "").trim(),
  }));
  if (!events.length) {
    ElMessage.error("至少需要一节课程；如果清空课表，请保留一节或删除该成员。");
    return;
  }
  for (const [index, event] of events.entries()) {
    if (!event.course) {
      ElMessage.error(`第 ${index + 1} 节缺少课程名称。`);
      return;
    }
    if (!event.start || !event.end) {
      ElMessage.error(`第 ${index + 1} 节缺少开始或结束时间。`);
      return;
    }
    if (event.end <= event.start) {
      ElMessage.error(`第 ${index + 1} 节的结束时间必须晚于开始时间。`);
      return;
    }
  }
  try {
    const saved = await apiPost<SavePageResult>("/api/schedule/save", {
      scope_id: schedule.scope_id,
      user_id: schedule.user_id,
      revision: schedule.revision,
      name: schedule.name.trim(),
      qq: (schedule.qq || "").trim(),
      events,
    });
    schedule.revision = saved.revision;
    schedule.name = saved.name;
    schedule.events = events;
    state.dirty = false;
    ElMessage.success(`已保存 ${saved.name} 的课表：${saved.event_count} 节课。`);
    await loadScopes();
  } catch (error) {
    if (error instanceof ApiError && error.status === 409) {
      ElMessage.error("课表已被其他操作更新，请点击「刷新数据」后重试。");
      return;
    }
    ElMessage.error(errorMessage(error));
  }
}

export async function loadOverrides(scopeId: string): Promise<void> {
  try {
    const data = await apiGet<{ overrides: DayOverrideRow[] }>("/api/overrides", {
      scope_id: scopeId,
    });
    if (state.selectedScopeId !== scopeId) return;
    state.overrides = data.overrides || [];
  } catch (error) {
    if (state.selectedScopeId !== scopeId) return;
    state.overrides = [];
    ElMessage.error(errorMessage(error));
  }
}

export async function setOverride(payload: {
  user_id: string;
  day: string;
  kind: string;
  source_day: string;
}): Promise<void> {
  const scopeId = state.selectedScopeId;
  if (!scopeId) return;
  await apiPost("/api/overrides/set", { scope_id: scopeId, ...payload });
  const kindText = payload.kind === "shift" ? "调休" : "休假";
  ElMessage.success(`已保存 ${payload.day} 的${kindText}标记。`);
  await loadOverrides(scopeId);
}

export async function deleteOverride(row: DayOverrideRow): Promise<void> {
  const scopeId = state.selectedScopeId;
  if (!scopeId) return;
  const kindText = row.kind === "shift" ? "调休" : "休假";
  try {
    await ElMessageBox.confirm(`删除 ${row.day} 的${kindText}标记（${row.name}）？`, "删除标记", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  try {
    await apiPost("/api/overrides/delete", {
      scope_id: scopeId,
      user_id: row.user_id,
      day: row.day,
    });
    ElMessage.success(`已删除 ${row.day} 的标记。`);
    await loadOverrides(scopeId);
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

export async function loadCheckins(scopeId: string): Promise<void> {
  try {
    const data = await apiGet<CheckinBoard>("/api/checkins", { scope_id: scopeId });
    if (state.selectedScopeId !== scopeId) return;
    state.checkinTotals = data.totals || [];
    state.checkinRecords = data.records || [];
  } catch (error) {
    if (state.selectedScopeId !== scopeId) return;
    state.checkinTotals = [];
    state.checkinRecords = [];
    ElMessage.error(errorMessage(error));
  }
}

export async function deleteCheckin(row: CheckinRecord): Promise<void> {
  const scopeId = state.selectedScopeId;
  if (!scopeId) return;
  try {
    await ElMessageBox.confirm(
      `删除 ${row.name || row.user_id} 在 ${row.day} 的签到记录（-${row.points} 分）？`,
      "删除签到记录",
      { type: "warning", confirmButtonText: "删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await apiPost("/api/checkins/delete", {
      scope_id: scopeId,
      user_id: row.user_id,
      day: row.day,
    });
    ElMessage.success(`已删除 ${row.day} 的签到记录。`);
    await loadCheckins(scopeId);
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

export async function loadPendingMembers(scopeId: string): Promise<{
  members: PendingMember[];
  note: string;
}> {
  const data = await apiGet<{ members: PendingMember[]; note: string }>("/api/members", {
    scope_id: scopeId,
  });
  return { members: data.members || [], note: data.note || "" };
}

export async function createMemberSchedules(
  scopeId: string,
  members: PendingMember[],
): Promise<number> {
  const result = await apiPost<{ created_count: number }>("/api/schedule/create", {
    scope_id: scopeId,
    members,
  });
  await loadScopes();
  return result.created_count;
}

export async function loadSettings(): Promise<BotSettings> {
  const data = await apiGet<{ settings: BotSettings }>("/api/settings");
  const settings = data.settings || ({} as BotSettings);
  return {
    // Treat an absent flag as on so an older payload never hides the bot.
    enabled: settings.enabled !== false,
    reply_plain: settings.reply_plain !== false,
    reply_slash: settings.reply_slash !== false,
    reply_mention: settings.reply_mention !== false,
    send_format: settings.send_format === "markdown" ? "markdown" : "image",
    nickname: settings.nickname || "",
  };
}

export async function saveSettings(settings: BotSettings): Promise<void> {
  await apiPost("/api/settings", settings);
}

export function importSummary(result: ImportResult | null): string {
  const summary = result || ({} as ImportResult);
  const parts = [
    `已导入 ${summary.member_count || 0} 位成员的课表（新增 ${summary.created_count || 0}、` +
      `覆盖 ${summary.updated_count || 0}），共 ${summary.event_count || 0} 节课程。`,
  ];
  if (summary.day_override_count) {
    parts.push(`同时恢复 ${summary.day_override_count} 条休假/调休标记。`);
  }
  const skipped = Array.isArray(summary.skipped) ? summary.skipped : [];
  if (skipped.length) {
    const shown = skipped.slice(0, 3).join("、");
    parts.push(`忽略 ${skipped.length} 个文件：${shown}${skipped.length > 3 ? "…" : ""}`);
  }
  return parts.join("");
}

export function safeFileLabel(value: string | undefined): string {
  const cleaned = String(value || "scope")
    .replace(/[\\/:*?"<>|\s]+/g, "_")
    .slice(0, 40);
  return cleaned || "scope";
}

export function timestamp(): string {
  const now = new Date();
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(
    now.getHours(),
  )}${pad(now.getMinutes())}`;
}

export async function exportMemberICS(): Promise<void> {
  const schedule = state.schedule;
  if (!schedule || !(await canExport())) return;
  const label = safeFileLabel(schedule.name || schedule.user_id);
  try {
    await downloadFile(
      "/api/export",
      { scope_id: schedule.scope_id, user_id: schedule.user_id, format: "ics" },
      `课表-${label}-${timestamp()}.ics`,
    );
    ElMessage.success("已开始下载课表文件。");
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function weekOccurrenceCount(events: PageSchedule["events"], weekStart: Date): number {
  return weekOccurrences(events || [], weekStart).length;
}

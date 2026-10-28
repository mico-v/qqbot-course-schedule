import { computed, reactive } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiError } from "../api/client";
import {
  createSchedules,
  deleteSchedule as removeSchedule,
  downloadICS,
  fetchOverrides,
  fetchPendingMembers,
  fetchSchedule,
  fetchScopes,
  fetchSettings,
  persistSettings,
  removeOverride,
  saveOverride,
  saveSchedule as persistSchedule,
} from "../api/admin";
import {
  buildScheduleSavePayload,
  importSummary as formatImportSummary,
  safeFileLabel as formatSafeFileLabel,
  timestampLabel,
  validateScheduleEvents,
} from "../domain/schedule";
import { normalizeSettings } from "../domain/settings";
import type {
  BotSettings,
  DayOverrideRow,
  ImportResult,
  PageSchedule,
  PendingMember,
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
  state.scopes = await fetchScopes();
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
  state.dirty = false;
}

let memberRequestId = 0;

export async function selectMember(scopeId: string, userId: string): Promise<void> {
  if (!(await canLeaveEditor())) return;
  const requestId = ++memberRequestId;
  try {
    const schedule = await fetchSchedule(scopeId, userId);
    if (requestId !== memberRequestId) return;
    state.schedule = schedule;
    state.selectedScopeId = scopeId;
    state.selectedUserId = userId;
    state.calendarWeekStart = initialWeekStart(schedule.events || []);
    state.dirty = false;
    await loadOverrides(scopeId);
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

export async function reloadSelectedMember(): Promise<void> {
  if (!state.selectedScopeId || !state.selectedUserId) return;
  const schedule = await fetchSchedule(state.selectedScopeId, state.selectedUserId);
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
      await loadOverrides(state.selectedScopeId);
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
  const payload = buildScheduleSavePayload(schedule);
  const validationError = validateScheduleEvents(payload.events);
  if (validationError) {
    ElMessage.error(validationError);
    return;
  }
  try {
    const saved = await persistSchedule(payload);
    schedule.revision = saved.revision;
    schedule.name = saved.name;
    schedule.events = payload.events;
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

export async function deleteMemberSchedule(): Promise<void> {
  const schedule = state.schedule;
  if (!schedule) return;
  try {
    await ElMessageBox.confirm(
      `确定删除 ${schedule.name || schedule.user_id} 的课表吗？全局模式下会同时清理可命中的跨群课表，删除后该成员会回到待创建列表。此操作不可撤销。`,
      "删除成员课表",
      {
        type: "warning",
        confirmButtonText: "确认删除",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    await removeSchedule({
      scope_id: schedule.scope_id,
      user_id: schedule.user_id,
      revision: schedule.revision,
    });
    state.schedule = null;
    state.selectedUserId = "";
    state.dirty = false;
    await loadScopes();
    ElMessage.success("成员课表已删除。");
  } catch (error) {
    if (error instanceof ApiError && error.status === 409) {
      ElMessage.error("课表已被其他操作更新，请刷新后重试。");
      return;
    }
    ElMessage.error(errorMessage(error));
  }
}

export async function loadOverrides(scopeId: string): Promise<void> {
  try {
    const rows = await fetchOverrides(scopeId);
    if (state.selectedScopeId !== scopeId) return;
    state.overrides = rows;
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
  await saveOverride({ scope_id: scopeId, ...payload });
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
    await removeOverride({
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

export async function loadPendingMembers(scopeId: string): Promise<{
  members: PendingMember[];
  note: string;
}> {
  return fetchPendingMembers(scopeId);
}

export async function createMemberSchedules(
  scopeId: string,
  members: PendingMember[],
): Promise<number> {
  const result = await createSchedules(scopeId, members);
  await loadScopes();
  return result.created_count;
}

export async function loadSettings(): Promise<BotSettings> {
  return normalizeSettings(await fetchSettings());
}

export async function saveSettings(settings: BotSettings): Promise<void> {
  await persistSettings(settings);
}

export function importSummary(result: ImportResult | null): string {
  return formatImportSummary(result);
}

export function safeFileLabel(value: string | undefined): string {
  return formatSafeFileLabel(value);
}

export function timestamp(): string {
  return timestampLabel();
}

export async function exportMemberICS(): Promise<void> {
  const schedule = state.schedule;
  if (!schedule || !(await canExport())) return;
  const label = safeFileLabel(schedule.name || schedule.user_id);
  try {
    await downloadICS(
      schedule.scope_id,
      schedule.user_id,
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

// 课表管理页面：与 /api/* 交互，保存时携带 revision 做乐观锁。
const $ = (selector) => document.querySelector(selector);

const state = {
  scopes: [],
  selectedScopeId: "",
  selectedUserId: "",
  schedule: null,
  overrides: [],
  dirty: false,
  calendarWeekStart: null,
};

const addMembers = {
  scopeId: "",
  label: "",
  members: [],
  selected: new Set(),
  filter: "",
  loading: false,
};

const transfer = {
  scopeId: "",
  label: "",
  memberCount: 0,
  file: null,
};

const notice = $("#notice");
const scopeList = $("#scopeList");
const weekCalendar = $("#weekCalendar");
const calendarScroll = $("#calendarScroll");

const WEEKDAY_CODES = ["MO", "TU", "WE", "TH", "FR", "SA", "SU"];
const WEEKDAY_LABELS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];
const CALENDAR_HOUR_HEIGHT = 54;
const CALENDAR_EVENT_COLORS = [
  "#4268df",
  "#0f8b8d",
  "#b45309",
  "#be123c",
  "#7c3aed",
  "#0369a1",
  "#4d7c0f",
  "#c2410c",
];
const courseEditor = {
  index: -1,
};

const THEME_STORAGE_KEY = "qqbot-admin-theme";
const THEME_MODES = ["auto", "light", "dark"];
const THEME_LABELS = {
  auto: "自动",
  light: "浅色",
  dark: "深色",
};
const systemTheme = window.matchMedia("(prefers-color-scheme: dark)");
const dialogState = {
  active: null,
  previousFocus: null,
};

function readThemeMode() {
  const documentMode = document.documentElement.dataset.themeMode;
  if (THEME_MODES.includes(documentMode)) return documentMode;
  try {
    const saved = window.localStorage.getItem(THEME_STORAGE_KEY);
    if (THEME_MODES.includes(saved)) return saved;
  } catch {
    /* Storage can be unavailable in privacy-restricted contexts. */
  }
  return "auto";
}

let themeMode = readThemeMode();

function effectiveTheme() {
  return themeMode === "auto" ? (systemTheme.matches ? "dark" : "light") : themeMode;
}

function applyTheme({ persist = false } = {}) {
  const root = document.documentElement;
  if (themeMode === "auto") delete root.dataset.theme;
  else root.dataset.theme = themeMode;
  root.dataset.themeMode = themeMode;

  const button = $("#themeButton");
  if (button) {
    const label = THEME_LABELS[themeMode];
    button.textContent = `主题：${label}`;
    button.title = `当前主题：${label}，点击切换`;
    button.setAttribute("aria-label", `当前主题：${label}，点击切换`);
  }

  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.content = effectiveTheme() === "dark" ? "#101722" : "#f3f6fb";

  if (persist) {
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, themeMode);
    } catch {
      /* Keep the current session usable even if persistence fails. */
    }
  }
}

function cycleTheme() {
  const index = THEME_MODES.indexOf(themeMode);
  themeMode = THEME_MODES[(index + 1) % THEME_MODES.length];
  applyTheme({ persist: true });
}

function setupTheme() {
  applyTheme();
  $("#themeButton")?.addEventListener("click", cycleTheme);
  const onSystemThemeChange = () => {
    if (themeMode === "auto") applyTheme();
  };
  if (typeof systemTheme.addEventListener === "function") {
    systemTheme.addEventListener("change", onSystemThemeChange);
  } else {
    systemTheme.addListener(onSystemThemeChange);
  }
}

const FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

function activeDialog() {
  return [...document.querySelectorAll(".dialog-overlay")].find(
    (dialog) => !dialog.classList.contains("hidden"),
  );
}

function focusableElements(dialog) {
  return [...dialog.querySelectorAll(FOCUSABLE_SELECTOR)].filter(
    (element) => element.getClientRects().length > 0,
  );
}

function setPageInert(inert) {
  const main = $("#mainContent");
  const skipLink = $(".skip-link");
  if (main) main.inert = inert;
  if (skipLink) skipLink.inert = inert;
  document.body.classList.toggle("dialog-open", inert);
}

function openDialog(dialog, initialFocusSelector) {
  dialogState.previousFocus =
    document.activeElement instanceof HTMLElement ? document.activeElement : null;
  dialogState.active = dialog;
  setPageInert(true);
  dialog.classList.remove("hidden");
  window.requestAnimationFrame(() => {
    const initial = initialFocusSelector ? dialog.querySelector(initialFocusSelector) : null;
    (initial || focusableElements(dialog)[0] || dialog).focus();
  });
}

function closeDialog(dialog) {
  dialog.classList.add("hidden");
  if (dialogState.active !== dialog) return;
  const previousFocus = dialogState.previousFocus;
  dialogState.active = null;
  dialogState.previousFocus = null;
  setPageInert(false);
  if (previousFocus && document.contains(previousFocus) && previousFocus.getClientRects().length) {
    previousFocus.focus();
  }
}

function handleDialogKeyboard(event) {
  const dialog = activeDialog();
  if (!dialog) return;
  if (event.key === "Escape") {
    event.preventDefault();
    closeDialog(dialog);
    return;
  }
  if (event.key !== "Tab") return;

  const focusable = focusableElements(dialog);
  if (!focusable.length) {
    event.preventDefault();
    dialog.focus();
    return;
  }

  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (!dialog.contains(document.activeElement)) {
    event.preventDefault();
    first.focus();
  } else if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  const text = await response.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!response.ok) {
    const error = new Error((data && data.error) || `请求失败（HTTP ${response.status}）`);
    error.status = response.status;
    throw error;
  }
  return data;
}

function apiGet(path, params = {}) {
  const query = new URLSearchParams(params).toString();
  return api(query ? `${path}?${query}` : path);
}

function apiPost(path, body) {
  return api(path, { method: "POST", body: JSON.stringify(body) });
}

function showNotice(message, type = "") {
  notice.textContent = message || "";
  notice.className = `notice${message ? " show" : ""}${type ? ` ${type}` : ""}`;
  notice.setAttribute("aria-live", type === "error" ? "assertive" : "polite");
}

function setDirty(value) {
  state.dirty = value;
  $("#dirtyMark").classList.toggle("hidden", !value);
}

function canLeaveEditor() {
  return !state.dirty || window.confirm("当前课表有未保存修改，确定要放弃吗？");
}

function setBusy(button, busy) {
  if (!button) return;
  button.disabled = busy;
  if (busy) {
    button.setAttribute("aria-busy", "true");
    button.dataset.oldText = button.textContent;
    button.textContent = "处理中…";
  } else {
    button.removeAttribute("aria-busy");
    button.textContent = button.dataset.oldText || button.textContent;
  }
}

function currentScope() {
  return state.scopes.find((scope) => scope.scope_id === state.selectedScopeId) || null;
}

function scopeMatches(scope, query) {
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

function makeMemberButton(scope, member) {
  const button = document.createElement("button");
  button.type = "button";
  const active =
    state.selectedScopeId === scope.scope_id && state.selectedUserId === member.user_id;
  button.className = `member-item${active ? " active" : ""}`;
  if (active) button.setAttribute("aria-current", "true");
  button.addEventListener("click", () => selectMember(scope.scope_id, member.user_id));

  const title = document.createElement("div");
  title.className = "member-item-title";
  title.textContent = member.name || member.user_id;
  const meta = document.createElement("div");
  meta.className = "member-item-meta";
  meta.textContent = `${member.user_id} · ${member.event_count || 0} 节课`;
  button.append(title, meta);
  return button;
}

function renderScopes() {
  const query = $("#scopeSearch").value.trim();
  const visible = state.scopes.filter((scope) => scopeMatches(scope, query));
  scopeList.textContent = "";
  const scopeCount = $("#scopeCount");
  scopeCount.textContent = String(query ? visible.length : state.scopes.length);
  scopeCount.title = query
    ? `搜索结果 ${visible.length} 个，共 ${state.scopes.length} 个会话`
    : `共 ${state.scopes.length} 个会话`;

  if (!visible.length) {
    const empty = document.createElement("p");
    empty.className = "scope-empty";
    empty.textContent = query ? "没有符合搜索条件的会话。" : "暂无可管理的会话。";
    scopeList.append(empty);
    return;
  }

  for (const scope of visible) {
    const wrapper = document.createElement("div");
    wrapper.className = "scope-item";
    const active = state.selectedScopeId === scope.scope_id;
    wrapper.classList.toggle("active", active);

    const header = document.createElement("div");
    header.className = "scope-heading";
    const scopeButton = document.createElement("button");
    scopeButton.type = "button";
    scopeButton.className = "scope-button";
    if (active) scopeButton.setAttribute("aria-current", "true");
    scopeButton.innerHTML = `<span class="scope-label"></span><span class="scope-meta"></span>`;
    scopeButton.querySelector(".scope-label").textContent = scope.label;
    const pendingCount = scope.pending_count || 0;
    scopeButton.querySelector(".scope-meta").textContent =
      scope.member_count > 0
        ? `${scope.member_count} 位成员 · ${scope.event_count} 节课`
        : pendingCount > 0
          ? `待添加 ${pendingCount} 位成员`
          : "0 位成员 · 0 节课";
    scopeButton.addEventListener("click", () => {
      if (!canLeaveEditor()) return;
      state.selectedScopeId = scope.scope_id;
      state.selectedUserId = "";
      state.schedule = null;
      $("#editor").classList.add("hidden");
      $("#emptyState").classList.remove("hidden");
      renderScopes();
    });
    header.append(scopeButton);

    if (scope.kind === "group") {
      const addButton = document.createElement("button");
      addButton.type = "button";
      addButton.className = "scope-add";
      addButton.title = "添加成员课表";
      addButton.setAttribute("aria-label", `向${scope.label}添加成员课表`);
      addButton.setAttribute("aria-haspopup", "dialog");
      addButton.textContent = "＋";
      addButton.addEventListener("click", (event) => {
        event.stopPropagation();
        openAddMembers(scope);
      });
      header.append(addButton);
    }
    const transferButton = document.createElement("button");
    transferButton.type = "button";
    transferButton.className = "scope-add";
    transferButton.title = `导入 / 导出「${scope.label}」`;
    transferButton.setAttribute("aria-label", `导入或导出${scope.label}`);
    transferButton.setAttribute("aria-haspopup", "dialog");
    transferButton.textContent = "⇅";
    transferButton.addEventListener("click", (event) => {
      event.stopPropagation();
      openTransfer(scope);
    });
    header.append(transferButton);
    wrapper.append(header);

    const members = document.createElement("div");
    members.className = "member-list";
    for (const member of scope.members || []) {
      members.append(makeMemberButton(scope, member));
    }
    wrapper.append(members);
    scopeList.append(wrapper);
  }
}

function renderEditor() {
  const schedule = state.schedule;
  const scope = currentScope();
  $("#emptyState").classList.add("hidden");
  $("#editor").classList.remove("hidden");
  $("#scopeLabel").textContent = scope ? scope.label : schedule.scope_id;
  $("#memberTitle").textContent = schedule.name || schedule.user_id;
  $("#memberMeta").textContent = `${schedule.events.length} 节课 · revision ${schedule.revision}`;
  $("#memberId").textContent = schedule.user_id;
  $("#memberName").value = schedule.name || "";
  $("#memberQQ").value = schedule.qq || "";
  updateAvatarPreview();
  state.calendarWeekStart = initialWeekStart(schedule.events || []);
  renderCalendar();
  setDirty(false);
}

function padNumber(value) {
  return String(value).padStart(2, "0");
}

function formatDateValue(date) {
  return `${date.getFullYear()}-${padNumber(date.getMonth() + 1)}-${padNumber(date.getDate())}`;
}

function formatTimeValue(date) {
  return `${padNumber(date.getHours())}:${padNumber(date.getMinutes())}`;
}

function formatClockTime(date) {
  return formatTimeValue(date);
}

function formatDateTimeValue(date) {
  return `${formatDateValue(date)}T${formatTimeValue(date)}`;
}

function parseLocalDateTime(value) {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(String(value || ""));
  if (!match) return null;
  const date = new Date(
    Number(match[1]),
    Number(match[2]) - 1,
    Number(match[3]),
    Number(match[4]),
    Number(match[5]),
  );
  return Number.isNaN(date.getTime()) ? null : date;
}

function startOfDay(date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function startOfWeek(date) {
  const day = startOfDay(date);
  const offset = (day.getDay() + 6) % 7;
  day.setDate(day.getDate() - offset);
  return day;
}

function addDays(date, days) {
  const result = startOfDay(date);
  result.setDate(result.getDate() + days);
  return result;
}

function calendarDayDiff(left, right) {
  const leftUTC = Date.UTC(left.getFullYear(), left.getMonth(), left.getDate());
  const rightUTC = Date.UTC(right.getFullYear(), right.getMonth(), right.getDate());
  return Math.round((rightUTC - leftUTC) / 86400000);
}

function sameCalendarDay(left, right) {
  return calendarDayDiff(left, right) === 0;
}

function weekdayCode(date) {
  return WEEKDAY_CODES[(date.getDay() + 6) % 7];
}

function parseRRule(value) {
  const fields = {};
  for (const part of String(value || "").split(";")) {
    const [key, fieldValue] = part.split("=", 2);
    if (key && fieldValue) fields[key.trim().toUpperCase()] = fieldValue.trim().toUpperCase();
  }
  return fields;
}

function parseRRuleUntil(value) {
  const raw = String(value || "").trim();
  let match = /^(\d{4})(\d{2})(\d{2})$/.exec(raw);
  if (match) {
    return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]), 23, 59, 59, 999);
  }
  match = /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z$/.exec(raw);
  if (match) {
    return new Date(
      Date.UTC(
        Number(match[1]),
        Number(match[2]) - 1,
        Number(match[3]),
        Number(match[4]),
        Number(match[5]),
        Number(match[6]),
      ),
    );
  }
  match = /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})$/.exec(raw);
  if (match) {
    return new Date(
      Number(match[1]),
      Number(match[2]) - 1,
      Number(match[3]),
      Number(match[4]),
      Number(match[5]),
      Number(match[6]),
    );
  }
  return null;
}

function rruleMatchesDate(rule, eventStart, candidate) {
  const frequency = rule.FREQ;
  if (!frequency) return sameCalendarDay(eventStart, candidate);
  const interval = Math.max(1, Number.parseInt(rule.INTERVAL || "1", 10) || 1);
  const dayDiff = calendarDayDiff(eventStart, candidate);
  if (dayDiff < 0) return false;

  if (frequency === "DAILY") return dayDiff % interval === 0;
  if (frequency === "WEEKLY") {
    const byDay = String(rule.BYDAY || weekdayCode(eventStart))
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean);
    if (!byDay.includes(weekdayCode(candidate))) return false;
    const weekDiff =
      calendarDayDiff(startOfWeek(eventStart), startOfWeek(candidate)) / 7;
    return weekDiff >= 0 && weekDiff % interval === 0;
  }
  if (frequency === "MONTHLY") {
    const monthDiff =
      (candidate.getFullYear() - eventStart.getFullYear()) * 12 +
      candidate.getMonth() -
      eventStart.getMonth();
    if (monthDiff < 0 || monthDiff % interval !== 0) return false;
    const byMonthDay = String(rule.BYMONTHDAY || eventStart.getDate())
      .split(",")
      .map((value) => Number.parseInt(value, 10));
    return byMonthDay.includes(candidate.getDate());
  }
  if (frequency === "YEARLY") {
    const yearDiff = candidate.getFullYear() - eventStart.getFullYear();
    if (yearDiff < 0 || yearDiff % interval !== 0) return false;
    const month = Number.parseInt(rule.BYMONTH || String(eventStart.getMonth() + 1), 10);
    const monthDay = Number.parseInt(rule.BYMONTHDAY || String(eventStart.getDate()), 10);
    return candidate.getMonth() + 1 === month && candidate.getDate() === monthDay;
  }
  return false;
}

function countRRuleOccurrences(rule, eventStart, candidate) {
  let count = 0;
  for (
    let day = startOfDay(eventStart);
    calendarDayDiff(day, candidate) >= 0;
    day = addDays(day, 1)
  ) {
    const candidateAt = new Date(
      day.getFullYear(),
      day.getMonth(),
      day.getDate(),
      eventStart.getHours(),
      eventStart.getMinutes(),
    );
    if (rruleMatchesDate(rule, eventStart, candidateAt)) count += 1;
    if (count > 10000) break;
  }
  return count;
}

function expandEventInWeek(event, eventIndex, weekStart) {
  const eventStart = parseLocalDateTime(event.start);
  const eventEnd = parseLocalDateTime(event.end);
  if (!eventStart || !eventEnd || eventEnd <= eventStart) return [];
  const duration = eventEnd.getTime() - eventStart.getTime();
  const weekEnd = addDays(weekStart, 7);
  const rule = parseRRule(event.rrule);
  const until = parseRRuleUntil(rule.UNTIL);
  const countLimit = Number.parseInt(rule.COUNT || "", 10);
  const occurrences = [];

  // Include the previous day so an overnight course remains visible in both days.
  for (let offset = -1; offset <= 6; offset += 1) {
    const day = addDays(weekStart, offset);
    const candidateStart = new Date(
      day.getFullYear(),
      day.getMonth(),
      day.getDate(),
      eventStart.getHours(),
      eventStart.getMinutes(),
    );
    if (!rruleMatchesDate(rule, eventStart, candidateStart)) continue;
    if (until && candidateStart > until) continue;
    if (countLimit && countRRuleOccurrences(rule, eventStart, candidateStart) > countLimit) {
      continue;
    }
    const candidateEnd = new Date(candidateStart.getTime() + duration);
    if (candidateEnd > weekStart && candidateStart < weekEnd) {
      occurrences.push({
        event,
        eventIndex,
        start: candidateStart,
        end: candidateEnd,
      });
    }
  }
  return occurrences;
}

function weekOccurrences(events, weekStart) {
  return events.flatMap((event, index) => expandEventInWeek(event, index, weekStart));
}

function initialWeekStart(events) {
  const currentWeek = startOfWeek(new Date());
  if (weekOccurrences(events, currentWeek).length) return currentWeek;
  const starts = events.map((event) => parseLocalDateTime(event.start)).filter(Boolean);
  starts.sort((left, right) => left - right);
  return starts.length ? startOfWeek(starts[0]) : currentWeek;
}

function calendarRange(occurrences) {
  let startMinute = 8 * 60;
  let endMinute = 20 * 60;
  for (const occurrence of occurrences) {
    const lastDay = startOfDay(new Date(occurrence.end.getTime() - 1));
    for (
      let day = startOfDay(occurrence.start);
      calendarDayDiff(day, lastDay) >= 0;
      day = addDays(day, 1)
    ) {
      const dayStart = startOfDay(day);
      const dayEnd = addDays(dayStart, 1);
      const sliceStart = new Date(Math.max(occurrence.start.getTime(), dayStart.getTime()));
      const sliceEnd = new Date(Math.min(occurrence.end.getTime(), dayEnd.getTime()));
      if (sliceEnd <= sliceStart) continue;
      const start = Math.round((sliceStart - dayStart) / 60000);
      const end = Math.round((sliceEnd - dayStart) / 60000);
      startMinute = Math.min(startMinute, Math.floor(start / 60) * 60);
      endMinute = Math.max(endMinute, Math.ceil(Math.max(end, start + 30) / 60) * 60);
    }
  }
  if (endMinute - startMinute < 8 * 60) endMinute = Math.min(24 * 60, startMinute + 8 * 60);
  return { startMinute, endMinute };
}

function eventColor(event) {
  const text = `${event.course || ""}${event.uid || ""}`;
  let hash = 0;
  for (let index = 0; index < text.length; index += 1) {
    hash = (hash * 31 + text.charCodeAt(index)) | 0;
  }
  return CALENDAR_EVENT_COLORS[Math.abs(hash) % CALENDAR_EVENT_COLORS.length];
}

function sliceOccurrencesForDay(occurrences, day) {
  const dayStart = startOfDay(day);
  const dayEnd = addDays(dayStart, 1);
  return occurrences.flatMap((occurrence) => {
    const start = new Date(Math.max(occurrence.start.getTime(), dayStart.getTime()));
    const end = new Date(Math.min(occurrence.end.getTime(), dayEnd.getTime()));
    if (end <= start) return [];
    return [
      {
        ...occurrence,
        start,
        end,
        continuesBefore: occurrence.start < dayStart,
        continuesAfter: occurrence.end > dayEnd,
      },
    ];
  });
}

function layoutDayOccurrences(occurrences) {
  const sorted = [...occurrences].sort(
    (left, right) => left.start - right.start || right.end - left.end,
  );
  const layout = new Map();
  let group = [];
  let groupEnd = 0;

  const flush = () => {
    if (!group.length) return;
    const columnEnds = [];
    for (const occurrence of group) {
      let column = columnEnds.findIndex((end) => end <= occurrence.start.getTime());
      if (column < 0) {
        column = columnEnds.length;
        columnEnds.push(0);
      }
      columnEnds[column] = occurrence.end.getTime();
      occurrence.column = column;
    }
    for (const occurrence of group) {
      layout.set(occurrence, { column: occurrence.column, columns: columnEnds.length });
    }
    group = [];
    groupEnd = 0;
  };

  for (const occurrence of sorted) {
    if (group.length && occurrence.start.getTime() >= groupEnd) flush();
    group.push(occurrence);
    groupEnd = Math.max(groupEnd, occurrence.end.getTime());
  }
  flush();
  return layout;
}

function updateEditorMeta() {
  if (!state.schedule) return;
  $("#memberMeta").textContent = `${state.schedule.events.length} 节课 · revision ${state.schedule.revision}`;
}

function renderCalendar() {
  const schedule = state.schedule;
  if (!schedule || !weekCalendar) return;
  schedule.events = Array.isArray(schedule.events) ? schedule.events : [];
  const weekStart = startOfWeek(state.calendarWeekStart || new Date());
  state.calendarWeekStart = weekStart;
  const weekEnd = addDays(weekStart, 6);
  const occurrences = weekOccurrences(schedule.events, weekStart);
  const range = calendarRange(occurrences);
  const totalMinutes = range.endMinute - range.startMinute;
  const minuteHeight = CALENDAR_HOUR_HEIGHT / 60;
  const bodyHeight = totalMinutes * minuteHeight;

  $("#calendarRange").textContent =
    weekStart.getFullYear() === weekEnd.getFullYear()
      ? `${weekStart.getFullYear()}年${weekStart.getMonth() + 1}月${weekStart.getDate()}日 - ${weekEnd.getMonth() + 1}月${weekEnd.getDate()}日`
      : `${weekStart.getFullYear()}年${weekStart.getMonth() + 1}月${weekStart.getDate()}日 - ${weekEnd.getFullYear()}年${weekEnd.getMonth() + 1}月${weekEnd.getDate()}日`;
  const count = $("#calendarCount");
  count.textContent = `${occurrences.length} 节`;
  count.setAttribute("aria-label", `本周 ${occurrences.length} 节课`);
  $("#noCourses").classList.toggle("hidden", occurrences.length > 0);
  updateEditorMeta();

  weekCalendar.textContent = "";
  weekCalendar.style.setProperty("--calendar-body-height", `${bodyHeight}px`);

  const corner = document.createElement("div");
  corner.className = "calendar-corner";
  corner.textContent = "时间";
  weekCalendar.append(corner);

  const dayHeads = document.createElement("div");
  dayHeads.className = "calendar-day-heads";
  for (let index = 0; index < 7; index += 1) {
    const day = addDays(weekStart, index);
    const head = document.createElement("div");
    head.className = "calendar-day-head";
    if (sameCalendarDay(day, new Date())) head.classList.add("today");
    const name = document.createElement("strong");
    name.textContent = WEEKDAY_LABELS[index];
    const date = document.createElement("span");
    date.textContent = `${day.getMonth() + 1}/${day.getDate()}`;
    head.append(name, date);
    dayHeads.append(head);
  }
  weekCalendar.append(dayHeads);

  const timeAxis = document.createElement("div");
  timeAxis.className = "calendar-time-axis";
  timeAxis.style.height = `${bodyHeight}px`;
  for (let minute = range.startMinute; minute <= range.endMinute; minute += 60) {
    const label = document.createElement("span");
    label.className = "calendar-time-label";
    label.style.top = `${(minute - range.startMinute) * minuteHeight}px`;
    label.textContent = `${padNumber(Math.floor(minute / 60) % 24)}:00`;
    timeAxis.append(label);
  }
  weekCalendar.append(timeAxis);

  const days = document.createElement("div");
  days.className = "calendar-days";
  days.style.height = `${bodyHeight}px`;
  for (let index = 0; index < 7; index += 1) {
    const dayDate = addDays(weekStart, index);
    const day = document.createElement("div");
    day.className = "calendar-day";
    if (sameCalendarDay(dayDate, new Date())) day.classList.add("today");
    day.dataset.date = formatDateValue(dayDate);
    day.dataset.startMinute = String(range.startMinute);
    day.dataset.endMinute = String(range.endMinute);
    day.style.height = `${bodyHeight}px`;
    day.tabIndex = 0;
    day.setAttribute(
      "aria-label",
      `${WEEKDAY_LABELS[index]}，${dayDate.getMonth() + 1}月${dayDate.getDate()}日，按回车添加课程`,
    );
    day.addEventListener("click", (event) => {
      if (event.target.closest(".calendar-event")) return;
      openCourseFromCalendar(dayDate, event);
    });
    day.addEventListener("keydown", (event) => {
      if (event.target !== day || (event.key !== "Enter" && event.key !== " ")) return;
      event.preventDefault();
      openCourseFromCalendar(dayDate, null);
    });

    const slices = sliceOccurrencesForDay(occurrences, dayDate);
    const layout = layoutDayOccurrences(slices);
    for (const occurrence of slices) {
      const position = layout.get(occurrence) || { column: 0, columns: 1 };
      const startMinute =
        Math.round((occurrence.start - dayDate) / 60000) - range.startMinute;
      const endMinute =
        Math.round((occurrence.end - dayDate) / 60000) - range.startMinute;
      const top = Math.max(0, startMinute * minuteHeight);
      const height = Math.max(34, (endMinute - startMinute) * minuteHeight);
      const eventButton = document.createElement("button");
      eventButton.type = "button";
      eventButton.className = "calendar-event";
      eventButton.dataset.eventIndex = String(occurrence.eventIndex);
      eventButton.style.top = `${top}px`;
      eventButton.style.height = `${height}px`;
      eventButton.style.left = `calc(${(position.column / position.columns) * 100}% + 3px)`;
      eventButton.style.width = `calc(${100 / position.columns}% - 6px)`;
      eventButton.style.setProperty("--event-accent", eventColor(occurrence.event));
      if (height < 66) eventButton.classList.add("compact");
      if (height < 46) eventButton.classList.add("tiny");

      const time = document.createElement("span");
      time.className = "calendar-event-time";
      time.textContent = `${formatClockTime(occurrence.start)} - ${formatClockTime(occurrence.end)}`;
      const title = document.createElement("strong");
      title.className = "calendar-event-title";
      title.textContent = occurrence.event.course || "未命名课程";
      const location = document.createElement("span");
      location.className = "calendar-event-location";
      location.textContent = occurrence.event.location || "";
      eventButton.append(time, title, location);
      eventButton.setAttribute(
        "aria-label",
        `${title.textContent}，${WEEKDAY_LABELS[index]} ${formatClockTime(occurrence.start)} 至 ${formatClockTime(occurrence.end)}${location.textContent ? `，地点 ${location.textContent}` : ""}，点击编辑`,
      );
      eventButton.title = `${title.textContent} ${formatClockTime(occurrence.start)}-${formatClockTime(occurrence.end)}`;
      eventButton.addEventListener("click", (event) => {
        event.stopPropagation();
        openCourseDialog(occurrence.eventIndex);
      });
      day.append(eventButton);
    }
    days.append(day);
  }
  weekCalendar.append(days);
}

function selectedWeekdays() {
  return [...document.querySelectorAll('input[name="courseWeekday"]:checked')].map(
    (input) => input.value,
  );
}

function setSelectedWeekdays(values) {
  const selected = new Set(values);
  for (const input of document.querySelectorAll('input[name="courseWeekday"]')) {
    input.checked = selected.has(input.value);
  }
}

function simpleRepeatValue(value) {
  const raw = String(value || "").trim();
  if (!raw) return "none";
  const rule = parseRRule(raw);
  if (!rule.FREQ) return "custom";
  const fields = Object.keys(rule);
  const hasSimpleInterval = !rule.INTERVAL || Number.parseInt(rule.INTERVAL, 10) === 1;
  const allowedFields =
    rule.FREQ === "WEEKLY"
      ? new Set(["FREQ", "INTERVAL", "BYDAY"])
      : new Set(["FREQ", "INTERVAL"]);
  if (!hasSimpleInterval || fields.some((field) => !allowedFields.has(field))) return "custom";
  if (rule.FREQ === "DAILY") return "daily";
  if (rule.FREQ === "WEEKLY") return "weekly";
  if (rule.FREQ === "MONTHLY") return "monthly";
  if (rule.FREQ === "YEARLY") return "yearly";
  return "custom";
}

function syncCourseRepeatFields({ initializeWeekdays = false } = {}) {
  const repeat = $("#courseRepeat").value;
  const weekly = repeat === "weekly";
  const custom = repeat === "custom";
  $("#courseWeekdaysField").classList.toggle("hidden", !weekly);
  $("#courseCustomRuleField").classList.toggle("hidden", !custom);
  $("#courseCustomRule").required = custom;
  if (weekly && initializeWeekdays && selectedWeekdays().length === 0) {
    const date = parseLocalDateTime(`${$("#courseDate").value}T00:00`);
    if (date) setSelectedWeekdays([weekdayCode(date)]);
  }
}

function courseRRule() {
  const repeat = $("#courseRepeat").value;
  if (repeat === "none") return "";
  if (repeat === "daily") return "FREQ=DAILY";
  if (repeat === "monthly") return "FREQ=MONTHLY";
  if (repeat === "yearly") return "FREQ=YEARLY";
  if (repeat === "custom") return $("#courseCustomRule").value.trim();

  let weekdays = selectedWeekdays();
  if (weekdays.length === 0) {
    const date = parseLocalDateTime(`${$("#courseDate").value}T00:00`);
    if (date) weekdays = [weekdayCode(date)];
  }
  return `FREQ=WEEKLY${weekdays.length ? `;BYDAY=${weekdays.join(",")}` : ""}`;
}

function timeValueFromMinutes(totalMinutes) {
  const normalized = Math.max(0, Math.min(23 * 60 + 59, Math.round(totalMinutes)));
  return `${padNumber(Math.floor(normalized / 60))}:${padNumber(normalized % 60)}`;
}

function openCourseFromCalendar(day, pointerEvent) {
  let startMinute = 8 * 60;
  const dayElement =
    pointerEvent?.currentTarget instanceof HTMLElement ? pointerEvent.currentTarget : null;
  if (pointerEvent && dayElement) {
    const rect = dayElement.getBoundingClientRect();
    const rangeStart = Number.parseInt(dayElement.dataset.startMinute || "480", 10);
    const offset = Math.max(0, pointerEvent.clientY - rect.top);
    startMinute = rangeStart + (offset / (CALENDAR_HOUR_HEIGHT / 60));
    startMinute = Math.round(startMinute / 30) * 30;
  }
  startMinute = Math.max(0, Math.min(23 * 60 + 30, startMinute));
  const endMinute = Math.min(23 * 60 + 59, startMinute + 60);
  openCourseDialog(-1, {
    date: formatDateValue(day),
    startTime: timeValueFromMinutes(startMinute),
    endTime: timeValueFromMinutes(endMinute),
  });
}

function openCourseDialog(index = -1, defaults = {}) {
  const events = state.schedule?.events || [];
  const event = index >= 0 && index < events.length ? events[index] : null;
  const now = new Date();
  const fallbackDate =
    state.calendarWeekStart && !sameCalendarDay(state.calendarWeekStart, startOfWeek(now))
      ? state.calendarWeekStart
      : now;
  const start = event ? parseLocalDateTime(event.start) : null;
  const end = event ? parseLocalDateTime(event.end) : null;
  const startTime = defaults.startTime || (start ? formatTimeValue(start) : "08:00");
  const endTime =
    defaults.endTime || (end ? formatTimeValue(end) : timeValueFromMinutes(9 * 60));

  courseEditor.index = event ? index : -1;
  $("#courseDialogTitle").textContent = event ? "编辑课程" : "添加课程";
  $("#courseDialogHint").textContent = event
    ? "修改后保存课程，原课程标识会保留。"
    : "设置首次上课日期、时间和重复规则。";
  $("#courseName").value = event?.course || "";
  $("#courseDate").value =
    defaults.date || (start ? formatDateValue(start) : formatDateValue(fallbackDate));
  $("#courseStartTime").value = startTime;
  $("#courseEndTime").value = endTime;
  $("#courseLocation").value = event?.location || "";
  $("#courseDescription").value = event?.description || "";
  $("#courseCustomRule").value = event?.rrule || "";
  $("#courseRepeat").value = event ? simpleRepeatValue(event.rrule) : "none";
  const rule = parseRRule(event?.rrule);
  const weekdays = String(rule.BYDAY || "")
    .split(",")
    .filter((value) => WEEKDAY_CODES.includes(value));
  if (weekdays.length === 0 && start && $("#courseRepeat").value === "weekly") {
    weekdays.push(weekdayCode(start));
  }
  setSelectedWeekdays(weekdays);
  syncCourseRepeatFields();
  $("#courseDelete").classList.toggle("hidden", !event);
  openDialog($("#courseDialog"), "#courseName");
}

function closeCourseDialog() {
  closeDialog($("#courseDialog"));
  courseEditor.index = -1;
}

function courseFormDateTime(date, time, addDay = false) {
  const parsedDate = parseLocalDateTime(`${date}T${time}`);
  if (!parsedDate) return null;
  if (addDay) parsedDate.setDate(parsedDate.getDate() + 1);
  return formatDateTimeValue(parsedDate);
}

function submitCourseForm(event) {
  event.preventDefault();
  const events = state.schedule?.events || [];
  const isEditing = courseEditor.index >= 0;
  const course = $("#courseName").value.trim();
  const date = $("#courseDate").value;
  const startTime = $("#courseStartTime").value;
  const endTime = $("#courseEndTime").value;
  if (!course || !date || !startTime || !endTime) {
    showNotice("请填写课程名称、日期和上下课时间。", "error");
    return;
  }
  if (startTime === endTime) {
    showNotice("结束时间必须晚于开始时间。", "error");
    $("#courseEndTime").focus();
    return;
  }
  const crossesMidnight = endTime < startTime;
  const start = courseFormDateTime(date, startTime);
  const end = courseFormDateTime(date, endTime, crossesMidnight);
  if (!start || !end) {
    showNotice("无法解析课程时间，请重新选择。", "error");
    return;
  }

  const rrule = courseRRule();
  if ($("#courseRepeat").value === "custom" && !rrule) {
    showNotice("请填写自定义重复规则。", "error");
    $("#courseCustomRule").focus();
    return;
  }
  const nextEvent = {
    id: courseEditor.index >= 0 ? events[courseEditor.index]?.id || 0 : 0,
    uid: courseEditor.index >= 0 ? events[courseEditor.index]?.uid || "" : "",
    course,
    start,
    end,
    location: $("#courseLocation").value.trim(),
    rrule,
    description: $("#courseDescription").value.trim(),
  };
  if (courseEditor.index >= 0) events[courseEditor.index] = nextEvent;
  else events.push(nextEvent);

  setDirty(true);
  closeCourseDialog();
  renderCalendar();
  showNotice(isEditing ? "课程已更新，保存课表后生效。" : "课程已添加，保存课表后生效。", "success");
}

function deleteCourse() {
  const events = state.schedule?.events || [];
  if (courseEditor.index < 0 || courseEditor.index >= events.length) return;
  const event = events[courseEditor.index];
  if (!window.confirm(`删除课程“${event.course || "未命名课程"}”？`)) return;
  if (events.length === 1) {
    showNotice("至少需要保留一节课程。", "error");
    return;
  }
  events.splice(courseEditor.index, 1);
  setDirty(true);
  closeCourseDialog();
  renderCalendar();
  showNotice("课程已删除，保存课表后生效。", "success");
}

function collectSchedule() {
  return (state.schedule?.events || []).map((event) => ({
    id: event.id || 0,
    uid: event.uid || "",
    course: String(event.course || "").trim(),
    start: String(event.start || "").trim(),
    end: String(event.end || "").trim(),
    location: String(event.location || "").trim(),
    rrule: String(event.rrule || "").trim(),
    description: String(event.description || "").trim(),
  }));
}

function validateSchedule(events) {
  if (!events.length) return "至少需要一节课程；如果清空课表，请保留一节或删除该成员。";
  for (const [index, event] of events.entries()) {
    if (!event.course) return `第 ${index + 1} 节缺少课程名称。`;
    if (!event.start || !event.end) return `第 ${index + 1} 节缺少开始或结束时间。`;
    if (event.end <= event.start) return `第 ${index + 1} 节的结束时间必须晚于开始时间。`;
  }
  return "";
}

async function selectMember(scopeId, userId) {
  if (!canLeaveEditor()) return;
  try {
    await loadMember(scopeId, userId);
    showNotice("");
  } catch (error) {
    showNotice(error.message, "error");
  }
}

async function loadMember(scopeId, userId) {
  const schedule = await apiGet("/api/schedule", { scope_id: scopeId, user_id: userId });
  state.schedule = schedule;
  state.selectedScopeId = scopeId;
  state.selectedUserId = userId;
  renderEditor();
  renderScopes();
  await loadOverrides(scopeId);
}

async function loadScopes({ keepSelection = true } = {}) {
  const data = await apiGet("/api/scopes");
  state.scopes = data.scopes || [];
  if (!keepSelection) {
    state.selectedScopeId = "";
    state.selectedUserId = "";
    state.schedule = null;
  }
  renderScopes();
}

function updateAvatarPreview() {
  const preview = $("#memberAvatarPreview");
  if (!preview) return;
  const qq = $("#memberQQ").value.trim();
  if (!/^[1-9][0-9]{4,10}$/.test(qq)) {
    preview.classList.add("hidden");
    preview.removeAttribute("src");
    return;
  }
  preview.src = `https://q1.qlogo.cn/g?b=qq&nk=${qq}&s=100`;
  preview.classList.remove("hidden");
}

async function saveSchedule() {
  if (!state.schedule) return;
  const events = collectSchedule();
  const invalid = validateSchedule(events);
  if (invalid) {
    showNotice(invalid, "error");
    return;
  }
  const button = $("#saveButton");
  setBusy(button, true);
  try {
    const saved = await apiPost("/api/schedule/save", {
      scope_id: state.schedule.scope_id,
      user_id: state.schedule.user_id,
      revision: state.schedule.revision,
      name: $("#memberName").value.trim(),
      qq: $("#memberQQ").value.trim(),
      events,
    });
    state.schedule.revision = saved.revision;
    state.schedule.name = saved.name;
    state.schedule.events = events;
    setDirty(false);
    renderCalendar();
    showNotice(`已保存 ${saved.name} 的课表：${saved.event_count} 节课。`, "success");
    await loadScopes();
  } catch (error) {
    if (error.status === 409) {
      showNotice("课表已被其他操作更新，请点击「刷新数据」后重试。", "error");
    } else {
      showNotice(error.message, "error");
    }
  } finally {
    setBusy(button, false);
  }
}

function visiblePicks() {
  const query = addMembers.filter.trim().toLocaleLowerCase();
  return addMembers.members.filter((member) => {
    if (!query) return true;
    return `${member.name} ${member.user_id}`.toLocaleLowerCase().includes(query);
  });
}

function updateSelectedMemberCount() {
  const count = $("#addMemberCount");
  const value = addMembers.selected.size;
  count.textContent = String(value);
  count.setAttribute("aria-label", `已选成员 ${value} 人`);
}

function makePickRow(member) {
  const row = document.createElement("label");
  const box = document.createElement("input");
  box.type = "checkbox";
  box.checked = addMembers.selected.has(member.user_id);
  row.className = `pick-row${box.checked ? " checked" : ""}`;
  box.addEventListener("change", () => {
    if (box.checked) addMembers.selected.add(member.user_id);
    else addMembers.selected.delete(member.user_id);
    row.classList.toggle("checked", box.checked);
    updateSelectedMemberCount();
  });
  const text = document.createElement("span");
  text.className = "pick-text";
  const name = document.createElement("span");
  name.className = "pick-name";
  name.textContent = member.name || "未命名成员";
  const meta = document.createElement("span");
  meta.className = "pick-meta";
  meta.textContent = member.user_id;
  text.append(name, meta);
  row.append(box, text);
  return row;
}

function renderPicker() {
  const list = $("#addMemberList");
  list.textContent = "";
  const visible = visiblePicks();
  for (const member of visible) {
    list.append(makePickRow(member));
  }
  updateSelectedMemberCount();
  const empty = $("#addMemberEmpty");
  if (!addMembers.loading && visible.length === 0) {
    empty.textContent = addMembers.members.length
      ? "没有符合搜索条件的成员。"
      : "暂无可添加的成员：官方群成员列表为内邀能力，这里显示在群里发过言或与机器人互动过、且还没有课表的成员。";
    empty.classList.remove("hidden");
  } else {
    empty.classList.add("hidden");
  }
}

function openAddMembers(scope) {
  addMembers.scopeId = scope.scope_id;
  addMembers.label = scope.label;
  addMembers.members = [];
  addMembers.selected = new Set();
  addMembers.filter = "";
  $("#addMemberSearch").value = "";
  $("#addMemberMeta").textContent = scope.label;
  openDialog($("#addMemberDialog"), "#addMemberSearch");
  loadAddMembers();
}

function closeAddMembers() {
  closeDialog($("#addMemberDialog"));
}

async function loadAddMembers() {
  addMembers.loading = true;
  $("#addMemberHint").textContent = "正在读取成员…";
  try {
    const data = await apiGet("/api/members", { scope_id: addMembers.scopeId });
    addMembers.members = data.members || [];
    addMembers.selected = new Set();
    if (data.note) $("#addMemberHint").textContent = data.note;
    else $("#addMemberHint").textContent = "为新成员创建空白课表后，即可在右侧编辑课程。";
  } catch (error) {
    addMembers.members = [];
    $("#addMemberHint").textContent = error.message;
  } finally {
    addMembers.loading = false;
    renderPicker();
  }
}

function toggleAllVisible() {
  const visible = visiblePicks();
  const allSelected = visible.every((member) => addMembers.selected.has(member.user_id));
  for (const member of visible) {
    if (allSelected) addMembers.selected.delete(member.user_id);
    else addMembers.selected.add(member.user_id);
  }
  renderPicker();
}

async function submitAddMembers() {
  if (addMembers.selected.size === 0) {
    showNotice("请先选择要添加课表的成员。", "error");
    return;
  }
  const button = $("#addMemberSubmit");
  setBusy(button, true);
  try {
    const members = addMembers.members.filter((member) => addMembers.selected.has(member.user_id));
    const result = await apiPost("/api/schedule/create", {
      scope_id: addMembers.scopeId,
      members,
    });
    closeAddMembers();
    showNotice(`已创建 ${result.created_count} 位成员的空白课表。`, "success");
    await loadScopes();
  } catch (error) {
    showNotice(error.message, "error");
  } finally {
    setBusy(button, false);
  }
}

async function refresh() {
  const keep = !state.dirty || canLeaveEditor();
  if (!keep) return;
  showNotice("");
  try {
    await loadScopes();
    if (state.selectedScopeId && state.selectedUserId) {
      await loadMember(state.selectedScopeId, state.selectedUserId);
    }
  } catch (error) {
    showNotice(error.message, "error");
  }
}

function canExport() {
  return !state.dirty || window.confirm("当前课表有未保存的修改，导出的是已保存的版本。确定继续吗？");
}

function safeFileLabel(value) {
  const cleaned = String(value || "scope").replace(/[\\/:*?"<>|\s]+/g, "_").slice(0, 40);
  return cleaned || "scope";
}

function timestamp() {
  const now = new Date();
  const pad = (value) => String(value).padStart(2, "0");
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`;
}

function formatSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function selectedMemberName(scopeId) {
  if (state.selectedScopeId !== scopeId || !state.selectedUserId) return "";
  const scope = state.scopes.find((item) => item.scope_id === scopeId);
  const member = scope?.members?.find((item) => item.user_id === state.selectedUserId);
  return member ? `${member.name || member.user_id}（${member.user_id}）` : "";
}

async function downloadExport(params, filename) {
  showNotice(`正在导出 ${filename}…`);
  const response = await fetch(`/api/export?${new URLSearchParams(params)}`);
  if (!response.ok) {
    let message = `导出失败（HTTP ${response.status}）`;
    try {
      const data = await response.json();
      if (data && data.error) message = data.error;
    } catch {
      /* not JSON */
    }
    throw new Error(message);
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
  showNotice(`已开始下载 ${filename}。`, "success");
}

async function exportScopeArchive(format) {
  if (!transfer.scopeId || !canExport()) return;
  const isBackup = format === "backup";
  const suffix = isBackup ? "原始备份" : "ICS";
  const extension = isBackup ? "json" : "zip";
  try {
    await downloadExport(
      { scope_id: transfer.scopeId, format },
      `课表-${suffix}-${safeFileLabel(transfer.label)}-${timestamp()}.${extension}`,
    );
  } catch (error) {
    showNotice(error.message, "error");
  }
}

function importSummary(result) {
  const summary = result || {};
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

function renderTransfer() {
  $("#transferMeta").textContent = `${transfer.label} · ${transfer.memberCount} 位成员`;
  $("#transferScopeCount").textContent = String(transfer.memberCount);
  $("#transferExportIcs").disabled = transfer.memberCount === 0;
  $("#transferExportBackup").disabled = transfer.memberCount === 0;
  $("#transferFileName").textContent = transfer.file
    ? `${transfer.file.name} · ${formatSize(transfer.file.size)}`
    : "未选择文件";
  $("#transferImport").disabled = !transfer.file;
  const member = selectedMemberName(transfer.scopeId);
  $("#transferHint").textContent = member
    ? `单个 .ics 会导入到当前选中的 ${member}；文件名形如 schedule<OpenID>.ics 时以文件名为准。`
    : "本会话还没有选中成员：导入单个 .ics 前请先选中成员，或把文件命名为 schedule<OpenID>.ics。";
}

function openTransfer(scope) {
  transfer.scopeId = scope.scope_id;
  transfer.label = scope.label;
  transfer.memberCount = scope.member_count || 0;
  transfer.file = null;
  $("#transferFile").value = "";
  renderTransfer();
  openDialog($("#transferDialog"), "#transferClose");
}

function closeTransfer() {
  closeDialog($("#transferDialog"));
  transfer.scopeId = "";
  transfer.label = "";
  transfer.memberCount = 0;
  transfer.file = null;
  $("#transferFile").value = "";
}

async function importArchive() {
  const file = transfer.file;
  if (!file || !transfer.scopeId) return;
  if (!canLeaveEditor()) return;
  const button = $("#transferImport");
  setBusy(button, true);
  showNotice("正在导入…");
  try {
    const form = new FormData();
    form.append("scope_id", transfer.scopeId);
    if (
      file.name.toLocaleLowerCase().endsWith(".ics") &&
      state.selectedScopeId === transfer.scopeId &&
      state.selectedUserId
    ) {
      form.append("user_id", state.selectedUserId);
    }
    form.append("file", file);
    const response = await fetch("/api/import", { method: "POST", body: form });
    const text = await response.text();
    let result = null;
    try {
      result = text ? JSON.parse(text) : null;
    } catch {
      result = null;
    }
    if (!response.ok) {
      throw new Error((result && result.error) || `导入失败（HTTP ${response.status}）`);
    }
    closeTransfer();
    await loadScopes();
    if (state.selectedScopeId && state.selectedUserId) {
      try {
        await loadMember(state.selectedScopeId, state.selectedUserId);
      } catch {
        /* member may have been replaced */
      }
    }
    showNotice(importSummary(result), "success");
  } catch (error) {
    showNotice(error.message, "error");
  } finally {
    setBusy(button, false);
  }
}

function overrideKindText(kind) {
  return kind === "shift" ? "调休" : "休假";
}

function todayValue() {
  const now = new Date();
  const pad = (value) => String(value).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

async function loadOverrides(scopeId) {
  try {
    const data = await apiGet("/api/overrides", { scope_id: scopeId });
    if (state.selectedScopeId !== scopeId) return;
    state.overrides = data.overrides || [];
  } catch (error) {
    if (state.selectedScopeId !== scopeId) return;
    state.overrides = [];
    showNotice(error.message, "error");
  }
  renderOverrides();
}

function renderOverrideMembers() {
  const select = $("#overrideMember");
  if (!select) return;
  const scope = currentScope();
  const previous = select.value;
  select.textContent = "";
  const all = document.createElement("option");
  all.value = "*";
  all.textContent = "全体成员";
  select.append(all);
  for (const member of scope?.members || []) {
    const option = document.createElement("option");
    option.value = member.user_id;
    option.textContent = member.name || member.user_id;
    select.append(option);
  }
  const preferred = previous || state.selectedUserId || "*";
  const exists = [...select.options].some((option) => option.value === preferred);
  select.value = exists ? preferred : "*";
}

function renderOverrides() {
  const list = $("#overrideList");
  if (!list) return;
  list.textContent = "";
  const rows = state.overrides || [];
  if (!rows.length) {
    const empty = document.createElement("p");
    empty.className = "no-courses";
    empty.textContent = "暂无休假/调休标记。";
    list.append(empty);
  }
  for (const row of rows) {
    const item = document.createElement("div");
    item.className = "override-item";

    const text = document.createElement("div");
    text.className = "override-item-text";
    const title = document.createElement("strong");
    title.textContent = `${row.day} · ${overrideKindText(row.kind)}`;
    const meta = document.createElement("span");
    meta.className = "muted";
    meta.textContent =
      row.kind === "shift" && row.source_day
        ? `${row.name} · 按 ${row.source_day} 的课程上课`
        : `${row.name} · 当天课程全部取消`;
    text.append(title, meta);

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "remove-course";
    remove.textContent = "删除";
    remove.addEventListener("click", () => deleteOverride(row));

    item.append(text, remove);
    list.append(item);
  }
  renderOverrideMembers();
}

function openOverrideForm() {
  $("#overrideForm").classList.remove("hidden");
  $("#overrideDay").value = $("#overrideDay").value || todayValue();
  $("#overrideKind").value = "holiday";
  $("#overrideSourceField").classList.add("hidden");
  $("#overrideSourceDay").value = "";
  renderOverrideMembers();
  $("#overrideMember").value = state.selectedUserId || "*";
}

function closeOverrideForm() {
  $("#overrideForm").classList.add("hidden");
}

async function submitOverride() {
  const scopeId = state.selectedScopeId;
  if (!scopeId) return;
  const day = $("#overrideDay").value;
  const kind = $("#overrideKind").value;
  const sourceDay = $("#overrideSourceDay").value;
  const userId = $("#overrideMember").value;
  if (!day) {
    showNotice("请选择标记日期。", "error");
    return;
  }
  if (kind === "shift" && !sourceDay) {
    showNotice("调休需要选择来源日期。", "error");
    return;
  }
  const button = $("#overrideSubmit");
  setBusy(button, true);
  try {
    await apiPost("/api/overrides/set", {
      scope_id: scopeId,
      user_id: userId,
      day,
      kind,
      source_day: sourceDay,
    });
    closeOverrideForm();
    showNotice(`已保存 ${day} 的${overrideKindText(kind)}标记。`, "success");
    await loadOverrides(scopeId);
  } catch (error) {
    showNotice(error.message, "error");
  } finally {
    setBusy(button, false);
  }
}

async function deleteOverride(row) {
  if (!state.selectedScopeId) return;
  if (!window.confirm(`删除 ${row.day} 的${overrideKindText(row.kind)}标记（${row.name}）？`)) return;
  try {
    await apiPost("/api/overrides/delete", {
      scope_id: state.selectedScopeId,
      user_id: row.user_id,
      day: row.day,
    });
    showNotice(`已删除 ${row.day} 的标记。`, "success");
    await loadOverrides(state.selectedScopeId);
  } catch (error) {
    showNotice(error.message, "error");
  }
}

async function exportMemberICS() {
  const schedule = state.schedule;
  if (!schedule || !canExport()) return;
  const label = safeFileLabel(schedule.name || schedule.user_id);
  try {
    await downloadExport(
      { scope_id: schedule.scope_id, user_id: schedule.user_id, format: "ics" },
      `课表-${label}-${timestamp()}.ics`,
    );
  } catch (error) {
    showNotice(error.message, "error");
  }
}

async function importMemberICS(file) {
  const schedule = state.schedule;
  if (!schedule || !file) return;
  if (!file.name.toLocaleLowerCase().endsWith(".ics")) {
    showNotice("单个成员只能导入 .ics 文件；.zip / .json 请在会话的导入/导出对话框中使用。", "error");
    return;
  }
  if (!canLeaveEditor()) return;
  showNotice(`正在导入 ${file.name}…`);
  try {
    const form = new FormData();
    form.append("scope_id", schedule.scope_id);
    form.append("user_id", schedule.user_id);
    form.append("file", file);
    const response = await fetch("/api/import", { method: "POST", body: form });
    const text = await response.text();
    let result = null;
    try {
      result = text ? JSON.parse(text) : null;
    } catch {
      result = null;
    }
    if (!response.ok) {
      throw new Error((result && result.error) || `导入失败（HTTP ${response.status}）`);
    }
    await loadScopes();
    await loadMember(schedule.scope_id, schedule.user_id);
    showNotice(importSummary(result), "success");
  } catch (error) {
    showNotice(error.message, "error");
  }
}

function settingsDialog() {
  return $("#settingsDialog");
}

async function loadSettings() {
  try {
    const data = await apiGet("/api/settings");
    const settings = (data && data.settings) || {};
    // Treat an absent flag as on so an older payload never hides the bot.
    $("#settingsEnabled").checked = settings.enabled !== false;
    $("#settingsReplyPlain").checked = settings.reply_plain !== false;
    $("#settingsReplySlash").checked = settings.reply_slash !== false;
    $("#settingsReplyMention").checked = settings.reply_mention !== false;
    $("#settingsSendFormat").value = settings.send_format === "markdown" ? "markdown" : "image";
  } catch (error) {
    showNotice(error.message, "error");
  }
}

function openSettings() {
  $("#settingsHint").textContent = "";
  openDialog(settingsDialog(), "#settingsEnabled");
  loadSettings();
}

function closeSettings() {
  closeDialog(settingsDialog());
}

async function saveSettings() {
  const button = $("#settingsSave");
  setBusy(button, true);
  try {
    await apiPost("/api/settings", {
      enabled: $("#settingsEnabled").checked,
      reply_plain: $("#settingsReplyPlain").checked,
      reply_slash: $("#settingsReplySlash").checked,
      reply_mention: $("#settingsReplyMention").checked,
      send_format: $("#settingsSendFormat").value,
    });
    $("#settingsHint").textContent = "已保存，立即生效。";
    showNotice("机器人设置已保存。", "success");
  } catch (error) {
    showNotice(error.message, "error");
  } finally {
    setBusy(button, false);
  }
}

function start() {
  setupTheme();
  document.addEventListener("keydown", handleDialogKeyboard);
  $("#refreshButton").addEventListener("click", refresh);
  $("#settingsButton").addEventListener("click", openSettings);
  $("#settingsClose").addEventListener("click", closeSettings);
  $("#settingsSave").addEventListener("click", saveSettings);
  settingsDialog().addEventListener("click", (event) => {
    if (event.target === settingsDialog()) closeSettings();
  });
  $("#scopeSearch").addEventListener("input", renderScopes);
  $("#addCourseButton").addEventListener("click", () => {
    openCourseDialog(-1);
  });
  $("#calendarPrev").addEventListener("click", () => {
    state.calendarWeekStart = addDays(state.calendarWeekStart || new Date(), -7);
    renderCalendar();
    calendarScroll.scrollTop = 0;
  });
  $("#calendarNext").addEventListener("click", () => {
    state.calendarWeekStart = addDays(state.calendarWeekStart || new Date(), 7);
    renderCalendar();
    calendarScroll.scrollTop = 0;
  });
  $("#calendarToday").addEventListener("click", () => {
    state.calendarWeekStart = startOfWeek(new Date());
    renderCalendar();
    calendarScroll.scrollTop = 0;
  });
  $("#courseForm").addEventListener("submit", submitCourseForm);
  $("#courseClose").addEventListener("click", closeCourseDialog);
  $("#courseCancel").addEventListener("click", closeCourseDialog);
  $("#courseDelete").addEventListener("click", deleteCourse);
  $("#courseRepeat").addEventListener("change", () => {
    syncCourseRepeatFields({ initializeWeekdays: true });
  });
  $("#courseDate").addEventListener("change", () => {
    if ($("#courseRepeat").value === "weekly" && selectedWeekdays().length === 0) {
      syncCourseRepeatFields({ initializeWeekdays: true });
    }
  });
  $("#courseDialog").addEventListener("click", (event) => {
    if (event.target === $("#courseDialog")) closeCourseDialog();
  });
  $("#memberName").addEventListener("input", () => setDirty(true));
  $("#memberQQ").addEventListener("input", () => {
    setDirty(true);
    updateAvatarPreview();
  });
  $("#memberAvatarPreview").addEventListener("error", (event) => {
    event.target.classList.add("hidden");
  });
  $("#saveButton").addEventListener("click", saveSchedule);
  $("#addMemberClose").addEventListener("click", closeAddMembers);
  $("#addMemberCancel").addEventListener("click", closeAddMembers);
  $("#addMemberReload").addEventListener("click", loadAddMembers);
  $("#addMemberSubmit").addEventListener("click", submitAddMembers);
  $("#addMemberSelectAll").addEventListener("click", toggleAllVisible);
  $("#addMemberSearch").addEventListener("input", (event) => {
    addMembers.filter = event.target.value;
    renderPicker();
  });
  $("#addMemberDialog").addEventListener("click", (event) => {
    if (event.target === $("#addMemberDialog")) closeAddMembers();
  });
  $("#transferClose").addEventListener("click", closeTransfer);
  $("#transferExportIcs").addEventListener("click", () => exportScopeArchive("ics"));
  $("#transferExportBackup").addEventListener("click", () => exportScopeArchive("backup"));
  $("#transferImport").addEventListener("click", importArchive);
  $("#transferFile").addEventListener("change", (event) => {
    transfer.file = event.target.files[0] || null;
    renderTransfer();
  });
  $("#transferDialog").addEventListener("click", (event) => {
    if (event.target === $("#transferDialog")) closeTransfer();
  });
  $("#memberExportButton").addEventListener("click", exportMemberICS);
  $("#memberImportButton").addEventListener("click", () => $("#memberImportFile").click());
  $("#memberImportFile").addEventListener("change", (event) => {
    const file = event.target.files[0] || null;
    event.target.value = "";
    importMemberICS(file);
  });
  $("#addOverrideButton").addEventListener("click", openOverrideForm);
  $("#overrideCancel").addEventListener("click", closeOverrideForm);
  $("#overrideSubmit").addEventListener("click", submitOverride);
  $("#overrideKind").addEventListener("change", (event) => {
    $("#overrideSourceField").classList.toggle("hidden", event.target.value !== "shift");
  });
  refresh();
}

start();

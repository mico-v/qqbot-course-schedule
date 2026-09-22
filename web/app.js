// 课表管理页面：与 /api/* 交互，保存时携带 revision 做乐观锁。
const $ = (selector) => document.querySelector(selector);

const state = {
  scopes: [],
  selectedScopeId: "",
  selectedUserId: "",
  schedule: null,
  overrides: [],
  dirty: false,
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
const courseList = $("#courseList");
const courseTemplate = $("#courseTemplate");

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
    button.dataset.oldText = button.textContent;
    button.textContent = "处理中…";
  } else {
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
  button.className = `member-item${
    state.selectedScopeId === scope.scope_id && state.selectedUserId === member.user_id ? " active" : ""
  }`;
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
  $("#scopeCount").textContent = String(state.scopes.length);

  for (const scope of visible) {
    const wrapper = document.createElement("div");
    wrapper.className = "scope-item";

    const header = document.createElement("div");
    header.className = "scope-heading";
    const scopeButton = document.createElement("button");
    scopeButton.type = "button";
    scopeButton.className = "scope-button";
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

  courseList.textContent = "";
  for (const event of schedule.events) {
    addCourseCard(event);
  }
  $("#noCourses").classList.toggle("hidden", schedule.events.length > 0);
  setDirty(false);
}

function addCourseCard(event = {}) {
  const fragment = courseTemplate.content.cloneNode(true);
  const card = fragment.querySelector(".course-card");
  const fields = {
    course: event.course || "",
    start: event.start || "",
    end: event.end || "",
    location: event.location || "",
    rrule: event.rrule || "",
    description: event.description || "",
  };
  for (const [name, value] of Object.entries(fields)) {
    const input = card.querySelector(`[data-field="${name}"]`);
    input.value = value;
    input.addEventListener("input", () => setDirty(true));
    input.addEventListener("change", () => setDirty(true));
  }
  card.querySelector(".remove-course").addEventListener("click", () => {
    card.remove();
    updateCourseIndexes();
    setDirty(true);
  });
  courseList.append(fragment);
  updateCourseIndexes();
  $("#noCourses").classList.add("hidden");
}

function updateCourseIndexes() {
  [...courseList.querySelectorAll(".course-card")].forEach((card, index) => {
    card.querySelector(".course-index").textContent = `第 ${index + 1} 节`;
  });
  $("#noCourses").classList.toggle("hidden", courseList.children.length > 0);
}

function collectSchedule() {
  return [...courseList.querySelectorAll(".course-card")].map((card) => {
    const value = (name) => card.querySelector(`[data-field="${name}"]`).value.trim();
    return {
      course: value("course"),
      start: value("start"),
      end: value("end"),
      location: value("location"),
      rrule: value("rrule"),
      description: value("description"),
    };
  });
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
      events,
    });
    state.schedule.revision = saved.revision;
    state.schedule.name = saved.name;
    state.schedule.events = events;
    setDirty(false);
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

function makePickRow(member) {
  const row = document.createElement("label");
  row.className = "member-pick";
  const box = document.createElement("input");
  box.type = "checkbox";
  box.checked = addMembers.selected.has(member.user_id);
  box.addEventListener("change", () => {
    if (box.checked) addMembers.selected.add(member.user_id);
    else addMembers.selected.delete(member.user_id);
    $("#addMemberCount").textContent = String(addMembers.selected.size);
  });
  const text = document.createElement("span");
  text.textContent = member.name ? `${member.name}（${member.user_id}）` : member.user_id;
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
  $("#addMemberCount").textContent = String(addMembers.selected.size);
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
  $("#addMemberDialog").classList.remove("hidden");
  loadAddMembers();
}

function closeAddMembers() {
  $("#addMemberDialog").classList.add("hidden");
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
  $("#transferDialog").classList.remove("hidden");
  renderTransfer();
}

function closeTransfer() {
  $("#transferDialog").classList.add("hidden");
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

function start() {
  $("#refreshButton").addEventListener("click", refresh);
  $("#scopeSearch").addEventListener("input", renderScopes);
  $("#addCourseButton").addEventListener("click", () => {
    addCourseCard({});
    setDirty(true);
  });
  $("#memberName").addEventListener("input", () => setDirty(true));
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

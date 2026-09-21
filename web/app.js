// 课表管理页面：与 /api/* 交互，保存时携带 revision 做乐观锁。
const $ = (selector) => document.querySelector(selector);

const state = {
  scopes: [],
  selectedScopeId: "",
  selectedUserId: "",
  schedule: null,
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
    scopeButton.querySelector(".scope-meta").textContent = `${scope.member_count} 位成员 · ${scope.event_count} 节课`;
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
      : "暂无可添加的成员：官方群成员列表为内邀能力，这里只显示与机器人互动过、且还没有课表的成员。";
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
  refresh();
}

start();

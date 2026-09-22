package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func seedScope(t *testing.T, service *schedule.Service, scope string) {
	t.Helper()
	if _, err := service.SaveICS(scope, "U1", "小明", adminTestICS, "schedule.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMemberSchedules(scope, []schedule.NewMember{{UserID: "U2", Name: "小红"}}, "webui"); err != nil {
		t.Fatal(err)
	}
}

func multipartRequest(t *testing.T, router *gin.Engine, path, scopeID, userID, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("scope_id", scopeID)
	if userID != "" {
		_ = writer.WriteField("user_id", userID)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, path, body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetBasicAuth("admin", adminPassword)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestExportImportArchiveAndBackup(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	seedScope(t, service, scope)

	// Export ICS archive.
	recorder := adminRequest(router, http.MethodGet, "/api/export?scope_id="+scope, nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("export ics = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Fatalf("export content type = %q", contentType)
	}
	archive := recorder.Body.Bytes()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	names := make(map[string]bool)
	for _, file := range reader.File {
		names[file.Name] = true
	}
	if !names["manifest.json"] {
		t.Fatalf("archive missing manifest: %v", names)
	}
	if !names["schedule_U1.ics"] {
		t.Fatalf("archive entries = %v", names)
	}
	if names["schedule_U2.ics"] {
		t.Fatalf("empty members must not appear in the ICS archive: %v", names)
	}

	// Import the archive into a fresh scope.
	target := schedule.ScopeGroup("G2")
	recorder = multipartRequest(t, router, "/api/import", target, "", "export.zip", archive)
	if recorder.Code != http.StatusOK {
		t.Fatalf("import zip = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var result schedule.ImportResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.MemberCount != 1 || result.CreatedCount != 1 || result.EventCount != 1 {
		t.Fatalf("import result = %+v", result)
	}
	page, found, _ := service.PageSchedule(target, "U1")
	if !found || len(page.Events) != 1 || page.Name != "小明" {
		t.Fatalf("imported page = %+v found=%v", page, found)
	}

	// Export backup JSON and restore it.
	recorder = adminRequest(router, http.MethodGet, "/api/export?scope_id="+scope+"&format=backup", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("export backup = %d", recorder.Code)
	}
	backupBytes := recorder.Body.Bytes()
	var backup schedule.BackupFile
	if err := json.Unmarshal(backupBytes, &backup); err != nil || backup.Version != 1 || len(backup.Members) != 2 {
		t.Fatalf("backup = %+v err=%v", backup, err)
	}
	recorder = multipartRequest(t, router, "/api/import", schedule.ScopeGroup("G3"), "", "backup.json", backupBytes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("import backup = %d (%s)", recorder.Code, recorder.Body.String())
	}

	// Single .ics import with an explicit member.
	recorder = multipartRequest(t, router, "/api/import", target, "U2", "我的课表.ics", []byte(adminTestICS))
	if recorder.Code != http.StatusOK {
		t.Fatalf("import single = %d (%s)", recorder.Code, recorder.Body.String())
	}
	page, found, _ = service.PageSchedule(target, "U2")
	if !found || len(page.Events) != 1 {
		t.Fatalf("single import page = %+v", page)
	}

	// A single .ics without a member and without the filename convention fails.
	recorder = multipartRequest(t, router, "/api/import", target, "", "random.ics", []byte(adminTestICS))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("single import without target = %d, want 400", recorder.Code)
	}

	// Unsupported extension.
	recorder = multipartRequest(t, router, "/api/import", target, "", "notes.txt", []byte("hello"))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("txt import = %d, want 400", recorder.Code)
	}
}

func TestImportFilenameConvention(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G9")
	recorder := multipartRequest(t, router, "/api/import", scope, "", "schedule_OPENID123.ics", []byte(adminTestICS))
	if recorder.Code != http.StatusOK {
		t.Fatalf("filename import = %d (%s)", recorder.Code, recorder.Body.String())
	}
	page, found, _ := service.PageSchedule(scope, "OPENID123")
	if !found || len(page.Events) != 1 {
		t.Fatalf("filename import page = %+v found=%v", page, found)
	}
}

var _ = io.Discard
var _ = strings.TrimSpace
var _ = time.Now

func TestExportSingleMemberICS(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	seedScope(t, service, scope)

	recorder := adminRequest(router, http.MethodGet, "/api/export?scope_id="+scope+"&user_id=U1", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("member export = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/calendar; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "BEGIN:VEVENT") || !strings.Contains(body, "高等数学") {
		t.Fatalf("body = %q", body)
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, ".ics") {
		t.Fatalf("disposition = %q", disposition)
	}

	// An empty member has nothing to export.
	if recorder := adminRequest(router, http.MethodGet, "/api/export?scope_id="+scope+"&user_id=U2", nil, true); recorder.Code != http.StatusBadRequest {
		t.Fatalf("empty member export = %d, want 400", recorder.Code)
	}
	// Unknown member.
	if recorder := adminRequest(router, http.MethodGet, "/api/export?scope_id="+scope+"&user_id=U9", nil, true); recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown member export = %d, want 404", recorder.Code)
	}
}

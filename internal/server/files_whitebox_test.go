package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schedule_ab12cd.ics"), []byte("BEGIN:VCALENDAR"), 0o644); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterFiles(router, dir)

	cases := []struct {
		path string
		want int
	}{
		{"/files/schedule_ab12cd.ics", http.StatusOK},
		{"/files/missing.ics", http.StatusNotFound},
		{"/files/..%2Fsecret.ics", http.StatusNotFound},
		{"/files/evil.txt", http.StatusNotFound},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != testCase.want {
			t.Errorf("%s = %d, want %d", testCase.path, recorder.Code, testCase.want)
		}
	}
}

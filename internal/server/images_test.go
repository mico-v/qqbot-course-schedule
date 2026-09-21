package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schedule_20260917_ab12.jpg"), []byte("jpeg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterImages(router, dir)

	cases := []struct {
		path string
		want int
	}{
		{"/images/schedule_20260917_ab12.jpg", http.StatusOK},
		{"/images/missing.jpg", http.StatusNotFound},
		{"/images/..%2Fsecret.jpg", http.StatusNotFound},
		{"/images/bad.txt", http.StatusNotFound},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != testCase.want {
			t.Errorf("%s status = %d, want %d", testCase.path, recorder.Code, testCase.want)
		}
	}
}

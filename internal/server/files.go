package server

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gin-gonic/gin"
)

var icsNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+\.ics$`)

// RegisterFiles serves exported .ics files (random names, short-lived).
func RegisterFiles(router *gin.Engine, dir string) {
	router.GET("/files/:name", func(c *gin.Context) {
		name := c.Param("name")
		if !icsNameRe.MatchString(name) {
			c.Status(http.StatusNotFound)
			return
		}
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Content-Type", "text/calendar; charset=utf-8")
		c.Header("Cache-Control", "public, max-age=3600")
		c.File(path)
	})
}

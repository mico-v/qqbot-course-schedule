// Package server hosts the public HTTP surface: health check, generated card
// images and (later) the admin API.
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gin-gonic/gin"
)

var imageNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+\.jpg$`)

// RegisterImages serves generated card images from dir.
func RegisterImages(router *gin.Engine, dir string) {
	router.GET("/images/:name", func(c *gin.Context) {
		name := c.Param("name")
		if !imageNameRe.MatchString(name) {
			c.Status(http.StatusNotFound)
			return
		}
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=3600")
		c.File(path)
	})
}

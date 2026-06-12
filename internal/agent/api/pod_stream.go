package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
)

func registerPodStreamRoutes(platform *gin.RouterGroup, client *k8sagent.Client) {
	platform.GET("/workloads/pods/:name/logs/stream", func(c *gin.Context) {
		namespace := c.DefaultQuery("namespace", "default")
		tailLines := int64(parseLimit(c.Query("tailLines"), 200))
		sinceSeconds := int64(parseLimit(c.Query("sinceSeconds"), 0))

		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		flusher, _ := c.Writer.(http.Flusher)
		writer := podLogStreamFlushWriter{writer: c.Writer, flusher: flusher}
		err := client.StreamPodLogs(c.Request.Context(), namespace, c.Param("name"), c.Query("container"), tailLines, sinceSeconds, writer)
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		if !c.Writer.Written() {
			writeError(c, err)
			return
		}
		_, _ = fmt.Fprintf(writer, "\n[agent] %v\n", err)
	})
}

type podLogStreamFlushWriter struct {
	writer  http.ResponseWriter
	flusher http.Flusher
}

func (w podLogStreamFlushWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return n, err
}

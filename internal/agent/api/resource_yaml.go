package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	apiresponse "github.com/opensoha/soha-agent/internal/api/response"
)

type resourceYAMLRequest struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Content   string `json:"content"`
}

type deleteResourceRequest struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

func registerResourceYAMLRoutes(platform *gin.RouterGroup, client *k8sagent.Client, actions actionPolicy) {
	platform.GET("/resources/yaml", func(c *gin.Context) {
		namespace := c.Query("namespace")
		kind := c.Query("kind")
		name := c.Query("name")
		if strings.TrimSpace(kind) == "" || strings.TrimSpace(name) == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "kind and name are required")
			return
		}
		item, err := client.GetResourceYAML(c.Request.Context(), namespace, kind, name)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.PUT("/resources/yaml", actions.Require(actionPlatformResourcesApply), func(c *gin.Context) {
		var req resourceYAMLRequest
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Kind) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Content) == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "kind, name, and content are required")
			return
		}
		item, err := client.ApplyResourceYAML(c.Request.Context(), req.Namespace, req.Kind, req.Name, req.Content)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.DELETE("/resources", actions.Require(actionPlatformResourcesDelete), func(c *gin.Context) {
		var req deleteResourceRequest
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Kind) == "" || strings.TrimSpace(req.Name) == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "kind and name are required")
			return
		}
		if err := client.DeleteResource(c.Request.Context(), req.Namespace, req.Kind, req.Name); err != nil {
			writeError(c, err)
			return
		}
		apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
}

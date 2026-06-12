package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	apiresponse "github.com/opensoha/soha-agent/internal/api/response"
	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
)

type customResourceListRequest struct {
	Definition domainresource.CRDResourceDefinition `json:"definition"`
	Namespace  string                               `json:"namespace"`
}

type customResourceYAMLRequest struct {
	Definition domainresource.CRDResourceDefinition `json:"definition"`
	Namespace  string                               `json:"namespace"`
	Name       string                               `json:"name,omitempty"`
	Content    string                               `json:"content,omitempty"`
}

func registerCustomResourceRoutes(platform *gin.RouterGroup, client *k8sagent.Client, actions actionPolicy) {
	platform.POST("/extensions/custom-resources/list", actions.Require(actionPlatformCustomResourcesList), func(c *gin.Context) {
		var req customResourceListRequest
		if err := c.ShouldBindJSON(&req); err != nil || invalidCRDDefinition(req.Definition) {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "valid custom resource definition is required")
			return
		}
		items, err := client.ListCustomResources(c.Request.Context(), req.Definition, req.Namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.POST("/extensions/custom-resources", actions.Require(actionPlatformCustomResourcesCreate), func(c *gin.Context) {
		var req customResourceYAMLRequest
		if err := c.ShouldBindJSON(&req); err != nil || invalidCRDDefinition(req.Definition) || strings.TrimSpace(req.Content) == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "definition and content are required")
			return
		}
		item, err := client.CreateCustomResourceYAML(c.Request.Context(), req.Definition, req.Namespace, req.Content)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusCreated, item)
	})
	platform.POST("/extensions/custom-resources/yaml", func(c *gin.Context) {
		var req customResourceYAMLRequest
		if err := c.ShouldBindJSON(&req); err != nil || invalidCRDDefinition(req.Definition) || strings.TrimSpace(req.Name) == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "definition and name are required")
			return
		}
		item, err := client.GetCustomResourceYAML(c.Request.Context(), req.Definition, req.Namespace, req.Name)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.PUT("/extensions/custom-resources/yaml", actions.Require(actionPlatformCustomResourcesApply), func(c *gin.Context) {
		var req customResourceYAMLRequest
		if err := c.ShouldBindJSON(&req); err != nil || invalidCRDDefinition(req.Definition) || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Content) == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "definition, name, and content are required")
			return
		}
		item, err := client.ApplyCustomResourceYAML(c.Request.Context(), req.Definition, req.Namespace, req.Name, req.Content)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.DELETE("/extensions/custom-resources", actions.Require(actionPlatformCustomResourcesDelete), func(c *gin.Context) {
		var req customResourceYAMLRequest
		if err := c.ShouldBindJSON(&req); err != nil || invalidCRDDefinition(req.Definition) || strings.TrimSpace(req.Name) == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "definition and name are required")
			return
		}
		if err := client.DeleteCustomResource(c.Request.Context(), req.Definition, req.Namespace, req.Name); err != nil {
			writeError(c, err)
			return
		}
		apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
}

func invalidCRDDefinition(definition domainresource.CRDResourceDefinition) bool {
	return strings.TrimSpace(definition.Group) == "" ||
		strings.TrimSpace(definition.Version) == "" ||
		strings.TrimSpace(definition.Resource) == "" ||
		strings.TrimSpace(definition.Kind) == ""
}

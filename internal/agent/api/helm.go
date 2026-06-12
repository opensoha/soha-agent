package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	apiresponse "github.com/opensoha/soha-agent/internal/api/response"
	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
)

type helmReleaseValuesRequest struct {
	Content string `json:"content"`
}

func registerHelmRoutes(platform *gin.RouterGroup, client *k8sagent.Client, actions actionPolicy) {
	platform.POST("/helm/charts/install", actions.Require(actionPlatformHelmReleaseInstall), func(c *gin.Context) {
		var req domainresource.HelmChartInstallInput
		if err := c.ShouldBindJSON(&req); err != nil || invalidHelmInstallInput(req) {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "repositoryUrl, chartName, version, releaseName, and namespace are required")
			return
		}
		item, err := client.InstallHelmChart(c.Request.Context(), req)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusCreated, item)
	})
	platform.PUT("/helm/releases/:name/values", actions.Require(actionPlatformHelmReleaseValuesUpdate), func(c *gin.Context) {
		var req helmReleaseValuesRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "invalid helm release values payload")
			return
		}
		namespace := strings.TrimSpace(c.Query("namespace"))
		name := strings.TrimSpace(c.Param("name"))
		if namespace == "" || name == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace and releaseName are required")
			return
		}
		item, err := client.UpdateHelmReleaseValues(c.Request.Context(), namespace, name, req.Content)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.DELETE("/helm/releases/:name", actions.Require(actionPlatformHelmReleaseDelete), func(c *gin.Context) {
		namespace := strings.TrimSpace(c.Query("namespace"))
		name := strings.TrimSpace(c.Param("name"))
		if namespace == "" || name == "" {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "namespace and releaseName are required")
			return
		}
		if err := client.DeleteHelmRelease(c.Request.Context(), namespace, name); err != nil {
			writeError(c, err)
			return
		}
		apiresponse.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
}

func invalidHelmInstallInput(input domainresource.HelmChartInstallInput) bool {
	return strings.TrimSpace(input.RepositoryURL) == "" ||
		strings.TrimSpace(input.ChartName) == "" ||
		strings.TrimSpace(input.Version) == "" ||
		strings.TrimSpace(input.ReleaseName) == "" ||
		strings.TrimSpace(input.Namespace) == ""
}

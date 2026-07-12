package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	apiresponse "github.com/opensoha/soha-agent/internal/api/response"
)

func registerPlatformConfigurationRoutes(platform *gin.RouterGroup, client *k8sagent.Client) {
	platform.GET("/configuration/configmaps", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListConfigMaps(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/secrets", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListSecrets(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/hpas", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListHorizontalPodAutoscalers(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/poddisruptionbudgets", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListPodDisruptionBudgets(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/priorityclasses", func(c *gin.Context) {
		items, err := client.ListPriorityClasses(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/runtimeclasses", func(c *gin.Context) {
		items, err := client.ListRuntimeClasses(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/mutatingwebhookconfigurations", func(c *gin.Context) {
		items, err := client.ListMutatingWebhookConfigurations(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/validatingwebhookconfigurations", func(c *gin.Context) {
		items, err := client.ListValidatingWebhookConfigurations(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/resourcequotas", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListResourceQuotas(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/limitranges", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListLimitRanges(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/configuration/leases", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListLeases(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/extensions/crds", func(c *gin.Context) {
		items, err := client.ListCRDs(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
}

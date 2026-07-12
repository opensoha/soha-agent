package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	apiresponse "github.com/opensoha/soha-agent/internal/api/response"
)

func registerPlatformRBACRoutes(platform *gin.RouterGroup, client *k8sagent.Client) {
	platform.GET("/access-control/serviceaccounts", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListServiceAccounts(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/access-control/serviceaccounts/:name/detail", func(c *gin.Context) {
		namespace := c.Query("namespace")
		item, err := client.GetServiceAccountDetail(c.Request.Context(), namespace, c.Param("name"))
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.GET("/access-control/roles", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListRoles(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/access-control/roles/:name/detail", func(c *gin.Context) {
		namespace := c.Query("namespace")
		item, err := client.GetRoleDetail(c.Request.Context(), namespace, c.Param("name"))
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.GET("/access-control/rolebindings", func(c *gin.Context) {
		namespace := c.Query("namespace")
		items, err := client.ListRoleBindings(c.Request.Context(), namespace)
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/access-control/rolebindings/:name/detail", func(c *gin.Context) {
		namespace := c.Query("namespace")
		item, err := client.GetRoleBindingDetail(c.Request.Context(), namespace, c.Param("name"))
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.GET("/access-control/clusterroles", func(c *gin.Context) {
		items, err := client.ListClusterRoles(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/access-control/clusterroles/:name/detail", func(c *gin.Context) {
		item, err := client.GetClusterRoleDetail(c.Request.Context(), c.Param("name"))
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
	platform.GET("/access-control/clusterrolebindings", func(c *gin.Context) {
		items, err := client.ListClusterRoleBindings(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Items(c, http.StatusOK, items)
	})
	platform.GET("/access-control/clusterrolebindings/:name/detail", func(c *gin.Context) {
		item, err := client.GetClusterRoleBindingDetail(c.Request.Context(), c.Param("name"))
		if err != nil {
			writeError(c, err)
			return
		}
		apiresponse.Item(c, http.StatusOK, item)
	})
}

package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	apiresponse "github.com/opensoha/soha-agent/internal/api/response"
	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
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
		namespace := strings.TrimSpace(c.Query("namespace"))
		subjectKind, subjectName, subjectNamespace, filtered := subjectFilter(c)
		if filtered && (namespace == "" || !validServiceAccountFilter(subjectKind, subjectName, subjectNamespace)) {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "a complete ServiceAccount subject filter is required")
			return
		}
		var items []domainresource.RoleBindingView
		var err error
		if filtered {
			items, err = client.ListRoleBindingsForSubject(c.Request.Context(), namespace, subjectKind, subjectName, subjectNamespace)
		} else {
			items, err = client.ListRoleBindings(c.Request.Context(), namespace)
		}
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
		subjectKind, subjectName, subjectNamespace, filtered := subjectFilter(c)
		if filtered && !validServiceAccountFilter(subjectKind, subjectName, subjectNamespace) {
			apiresponse.Error(c, http.StatusBadRequest, "invalid_argument", "a complete ServiceAccount subject filter is required")
			return
		}
		var items []domainresource.ClusterRoleBindingView
		var err error
		if filtered {
			items, err = client.ListClusterRoleBindingsForSubject(c.Request.Context(), subjectKind, subjectName, subjectNamespace)
		} else {
			items, err = client.ListClusterRoleBindings(c.Request.Context())
		}
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

func subjectFilter(c *gin.Context) (kind, name, namespace string, requested bool) {
	kind = strings.TrimSpace(c.Query("subjectKind"))
	name = strings.TrimSpace(c.Query("subjectName"))
	namespace = strings.TrimSpace(c.Query("subjectNamespace"))
	requested = kind != "" || name != "" || namespace != ""
	return kind, name, namespace, requested
}

func validServiceAccountFilter(kind, name, namespace string) bool {
	return kind == "ServiceAccount" && name != "" && namespace != ""
}

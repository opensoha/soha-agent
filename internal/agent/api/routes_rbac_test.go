package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRoleBindingSubjectFilterRequiresCompleteServiceAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPlatformRBACRoutes(router.Group(""), nil)
	request := httptest.NewRequest(
		http.MethodGet,
		"/access-control/rolebindings?namespace=platform&subjectKind=ServiceAccount&subjectName=app",
		nil,
	)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

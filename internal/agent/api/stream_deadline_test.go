package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.deadline = deadline
	return nil
}

func TestClearResponseWriteDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder(), deadline: time.Now()}
	ctx, _ := gin.CreateTestContext(recorder)

	if err := clearResponseWriteDeadline(ctx); err != nil {
		t.Fatalf("clearResponseWriteDeadline() error = %v", err)
	}
	if !recorder.deadline.IsZero() {
		t.Fatalf("deadline = %v, want zero", recorder.deadline)
	}
}

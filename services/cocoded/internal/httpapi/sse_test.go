package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWriteSSE(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/events", func(c *gin.Context) {
		if err := WriteSSE(c, SSEEvent{
			ID:    "1",
			Event: "ReviewStarted",
			Data: map[string]string{
				"session_id": "session-1",
			},
		}); err != nil {
			t.Fatalf("write sse: %v", err)
		}
	})

	request := httptest.NewRequest(http.MethodGet, "/events", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	body := response.Body.String()
	for _, expected := range []string{
		"id: 1\n",
		"event: ReviewStarted\n",
		`data: {"session_id":"session-1"}`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected SSE body to contain %q, got %q", expected, body)
		}
	}
}

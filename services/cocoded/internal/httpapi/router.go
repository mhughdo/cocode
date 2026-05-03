package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Data      any    `json:"data"`
	Error     any    `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

func NewRouter(version string) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, Envelope{
			Data: HealthResponse{
				Status:  "ok",
				Service: "cocoded",
				Version: version,
			},
			Error: nil,
		})
	})

	return router
}

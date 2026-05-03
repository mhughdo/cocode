package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
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

type VersionResponse struct {
	Service string `json:"service"`
	Version string `json:"version"`
	DataDir string `json:"data_dir"`
}

func NewRouter(config app.Config, logger *slog.Logger) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestIDMiddleware())
	router.Use(loggingMiddleware(logger))

	router.GET("/api/health", func(c *gin.Context) {
		respondOK(c, HealthResponse{
			Status:  "ok",
			Service: "cocoded",
			Version: config.Version,
		})
	})

	router.GET("/api/version", func(c *gin.Context) {
		respondOK(c, VersionResponse{
			Service: "cocoded",
			Version: config.Version,
			DataDir: config.DataDir,
		})
	})

	api := router.Group("/api")
	api.Use(authMiddleware(config.AuthToken))
	api.GET("/session", func(c *gin.Context) {
		respondOK(c, gin.H{
			"status": "authenticated",
		})
	})

	return router
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Data:      data,
		Error:     nil,
		RequestID: requestID(c),
	})
}

func respondError(c *gin.Context, err *apperror.Error) {
	c.JSON(err.Status, Envelope{
		Data:      nil,
		Error:     err,
		RequestID: requestID(c),
	})
}

func authMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if subtleTokenMatch(c.GetHeader("X-Cocode-Token"), token) ||
			subtleTokenMatch(bearerToken(c.GetHeader("Authorization")), token) {
			c.Next()
			return
		}

		respondError(c, apperror.Unauthorized("missing or invalid local auth token"))
		c.Abort()
	}
}

func bearerToken(header string) string {
	value := strings.TrimSpace(header)
	if value == "" {
		return ""
	}

	prefix := "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}

func subtleTokenMatch(got string, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}

	var result byte
	for i := range got {
		result |= got[i] ^ want[i]
	}
	return result == 0
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func loggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		logger.Info(
			"http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"request_id", requestID(c),
		)
	}
}

func requestID(c *gin.Context) string {
	value, ok := c.Get("request_id")
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(bytes[:])
}

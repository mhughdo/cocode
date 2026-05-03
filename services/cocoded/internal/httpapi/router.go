package httpapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
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

type ChangedFileResponse struct {
	ID              string          `json:"id"`
	SnapshotID      string          `json:"snapshot_id"`
	Path            string          `json:"path"`
	OldPath         string          `json:"old_path,omitempty"`
	Status          string          `json:"status"`
	Additions       int64           `json:"additions"`
	Deletions       int64           `json:"deletions"`
	IsBinary        bool            `json:"is_binary"`
	IsGenerated     bool            `json:"is_generated"`
	IsExcluded      bool            `json:"is_excluded"`
	LineRanges      json.RawMessage `json:"line_ranges"`
	PatchArtifactID string          `json:"patch_artifact_id,omitempty"`
}

func NewRouter(config app.Config, logger *slog.Logger, database *sql.DB) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	queries := dbgen.New(database)
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
	api.GET("/pr-snapshots/:id/changed-files", changedFilesHandler(queries))

	return router
}

func changedFilesHandler(queries *dbgen.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshotID := strings.TrimSpace(c.Param("id"))
		if snapshotID == "" {
			respondError(c, apperror.InvalidRequest("snapshot id is required"))
			return
		}
		if _, err := queries.GetPullRequestSnapshot(c.Request.Context(), snapshotID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(c, apperror.NotFound("snapshot was not found"))
				return
			}
			respondError(c, apperror.Internal("failed to read snapshot"))
			return
		}

		files, err := queries.ListChangedFilesBySnapshot(c.Request.Context(), snapshotID)
		if err != nil {
			respondError(c, apperror.Internal("failed to list changed files"))
			return
		}
		response := make([]ChangedFileResponse, 0, len(files))
		for _, file := range files {
			item, err := changedFileResponse(file)
			if err != nil {
				respondError(c, apperror.Internal("changed file line ranges are invalid"))
				return
			}
			response = append(response, item)
		}
		respondOK(c, response)
	}
}

func changedFileResponse(file dbgen.ChangedFile) (ChangedFileResponse, error) {
	lineRanges := json.RawMessage(file.LineRangesJson)
	if len(lineRanges) == 0 {
		lineRanges = json.RawMessage("[]")
	}
	if !json.Valid(lineRanges) {
		return ChangedFileResponse{}, errors.New("invalid line ranges JSON")
	}
	return ChangedFileResponse{
		ID:              file.ID,
		SnapshotID:      file.SnapshotID,
		Path:            file.Path,
		OldPath:         nullableResponseString(file.OldPath),
		Status:          file.Status,
		Additions:       file.Additions,
		Deletions:       file.Deletions,
		IsBinary:        file.IsBinary != 0,
		IsGenerated:     file.IsGenerated != 0,
		IsExcluded:      file.IsExcluded != 0,
		LineRanges:      lineRanges,
		PatchArtifactID: nullableResponseString(file.PatchArtifactID),
	}, nil
}

func nullableResponseString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
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

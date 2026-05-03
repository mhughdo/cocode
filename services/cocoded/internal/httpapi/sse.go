package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SSEEvent struct {
	ID    string
	Event string
	Data  any
}

func WriteSSE(c *gin.Context, event SSEEvent) error {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	if event.ID != "" {
		if _, err := fmt.Fprintf(c.Writer, "id: %s\n", event.ID); err != nil {
			return err
		}
	}

	if event.Event != "" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", event.Event); err != nil {
			return err
		}
	}

	payload, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
		return err
	}

	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

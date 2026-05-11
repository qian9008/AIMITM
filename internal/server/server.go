package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"airoxy-linux/internal/config"
	"airoxy-linux/internal/proxy"
	"github.com/gin-gonic/gin"
)

type Event struct {
	Time      string `json:"time"`
	Host      string `json:"host"`
	Rule      string `json:"rule"`
	Upstream  string `json:"upstream"`
	Duration  string `json:"duration"`
	Status    int    `json:"status"`
}

var (
	clients    = make(map[chan Event]bool)
	clientsMu  sync.RWMutex
	eventChan  = make(chan Event, 100)
)

func init() {
	go broadcastEvents()
}

func broadcastEvents() {
	for event := range eventChan {
		clientsMu.RLock()
		for clientChan := range clients {
			select {
			case clientChan <- event:
			default:
			}
		}
		clientsMu.RUnlock()
	}
}

func StartAdmin(port int) {
	gin.SetMode(gin.ReleaseMode)
	
	r := gin.New()
	r.Use(gin.Recovery())

	r.Static("/ui", "./static")

	api := r.Group("/api")
	{
		api.GET("/ca/download", func(c *gin.Context) {
			c.Header("Content-Disposition", "attachment; filename=rootCA.crt")
			c.Data(http.StatusOK, "application/x-x509-ca-cert", proxy.GetCACert())
		})

		api.GET("/config", func(c *gin.Context) {
			c.JSON(http.StatusOK, config.Get())
		})

		api.POST("/config", func(c *gin.Context) {
			var newConfig config.Config
			if err := c.ShouldBindJSON(&newConfig); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := config.Update(newConfig); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		api.GET("/events", sseHandler)

		api.GET("/status", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"memory_usage": "Optimized",
				"connections":  len(clients),
			})
		})
	}

	r.Run(fmt.Sprintf(":%d", port))
}

func sseHandler(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	clientChan := make(chan Event, 10)
	clientsMu.Lock()
	clients[clientChan] = true
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, clientChan)
		clientsMu.Unlock()
		close(clientChan)
	}()

	c.Stream(func(w io.Writer) bool {
		if event, ok := <-clientChan; ok {
			data, _ := json.Marshal(event)
			c.SSEvent("message", string(data))
			return true
		}
		return false
	})
}

func PushEvent(host, rule, upstream string, duration time.Duration, status int) {
	select {
	case eventChan <- Event{
		Time:     time.Now().Format("15:04:05"),
		Host:     host,
		Rule:     rule,
		Upstream: upstream,
		Duration: duration.String(),
		Status:   status,
	}:
	default:
	}
}

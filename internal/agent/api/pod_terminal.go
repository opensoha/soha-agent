package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	k8sagent "github.com/opensoha/soha-agent/internal/agent/kubernetes"
	"k8s.io/client-go/tools/remotecommand"
)

type podTerminalMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

func registerPodTerminalRoutes(platform *gin.RouterGroup, client *k8sagent.Client, actions actionPolicy) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	platform.GET("/workloads/pods/:name/terminal", actions.Require(actionPlatformPodsExec), func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		stdinReader, stdinWriter := io.Pipe()
		defer stdinWriter.Close()
		sizeQueue := newPodTerminalSizeQueue()
		defer sizeQueue.Close()

		var writeMu sync.Mutex
		stdout := podTerminalStreamWriter{conn: conn, writeMu: &writeMu, channel: "stdout"}
		stderr := podTerminalStreamWriter{conn: conn, writeMu: &writeMu, channel: "stderr"}
		streamErrCh := make(chan error, 1)
		go func() {
			streamErrCh <- client.StreamPodTerminal(
				ctx,
				c.DefaultQuery("namespace", "default"),
				c.Param("name"),
				c.Query("container"),
				normalizePodTerminalShell(c.DefaultQuery("shell", "/bin/sh")),
				stdinReader,
				stdout,
				stderr,
				sizeQueue,
			)
		}()

		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			defer stdinWriter.Close()
			for {
				var message podTerminalMessage
				if err := conn.ReadJSON(&message); err != nil {
					cancel()
					return
				}
				switch message.Type {
				case "input":
					if _, err := io.WriteString(stdinWriter, message.Data); err != nil {
						cancel()
						return
					}
				case "resize":
					sizeQueue.Push(message.Cols, message.Rows)
				case "close":
					cancel()
					return
				}
			}
		}()

		streamErr := <-streamErrCh
		cancel()
		<-readDone
		if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
			_ = writePodTerminalMessage(conn, &writeMu, podTerminalMessage{Type: "error", Message: streamErr.Error()})
			return
		}
		_ = writePodTerminalMessage(conn, &writeMu, podTerminalMessage{Type: "exit", Message: "terminal session closed"})
	})
}

type podTerminalStreamWriter struct {
	conn    *websocket.Conn
	writeMu *sync.Mutex
	channel string
}

func (w podTerminalStreamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := writePodTerminalMessage(w.conn, w.writeMu, podTerminalMessage{Type: w.channel, Data: string(p)}); err != nil {
		return 0, err
	}
	return len(p), nil
}

type podTerminalSizeQueue struct {
	ch   chan remotecommand.TerminalSize
	once sync.Once
}

func newPodTerminalSizeQueue() *podTerminalSizeQueue {
	return &podTerminalSizeQueue{ch: make(chan remotecommand.TerminalSize, 1)}
}

func (q *podTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.ch
	if !ok {
		return nil
	}
	return &size
}

func (q *podTerminalSizeQueue) Push(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	size := remotecommand.TerminalSize{Width: uint16(cols), Height: uint16(rows)}
	select {
	case q.ch <- size:
	default:
		select {
		case <-q.ch:
		default:
		}
		q.ch <- size
	}
}

func (q *podTerminalSizeQueue) Close() {
	q.once.Do(func() {
		close(q.ch)
	})
}

func writePodTerminalMessage(conn *websocket.Conn, writeMu *sync.Mutex, message podTerminalMessage) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return conn.WriteJSON(message)
}

func normalizePodTerminalShell(shell string) string {
	switch strings.TrimSpace(shell) {
	case "/bin/bash":
		return "/bin/bash"
	case "/bin/ash":
		return "/bin/ash"
	case "/busybox/sh":
		return "/busybox/sh"
	default:
		return "/bin/sh"
	}
}

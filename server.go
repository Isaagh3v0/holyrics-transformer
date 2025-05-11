package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var (
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
)

func StartServer() {
	port := GetEnv("PORT", "3000")
	lyricsURL := GetEnv("LYRICS_URL", "localhost")
	lyricsPort := GetEnv("LYRICS_PORT", "4747")

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			Error(fmt.Sprintf("WebSocket upgrade error: %v", err))
			return
		}

		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()

		Info(fmt.Sprintf("New connection: %s", conn.RemoteAddr().String()))

		mutex.Lock()
		if lastText != "" {
			content := strings.Split(lastText, "\n")
			payload := map[string]interface{}{
				"type":    lastType,
				"header":  lastHeader,
				"content": content,
			}
			err := conn.WriteJSON(payload)
			if err != nil {
				Error(fmt.Sprintf("Ошибка отправки начального текста клиенту %s: %v", conn.RemoteAddr().String(), err))
				conn.Close()
				clientsMu.Lock()
				delete(clients, conn)
				clientsMu.Unlock()
				return
			}
			Info(fmt.Sprintf("Отправлен последний текст новому клиенту: %s", conn.RemoteAddr().String()))
		}
		mutex.Unlock()

		defer func() {
			clientsMu.Lock()
			delete(clients, conn)
			clientsMu.Unlock()
			conn.Close()
			Info(fmt.Sprintf("Disconnected: %s", conn.RemoteAddr().String()))
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					Error(fmt.Sprintf("WebSocket error: %v", err))
				}
				break
			}
		}
	})

	http.Handle("/", http.FileServer(http.Dir("./public")))

	go startTextPolling(nil, lyricsURL, lyricsPort)

	Info(fmt.Sprintf("Server is running on port %s...", port))
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		Error(fmt.Sprintf("Server error: %v", err))
	}
}

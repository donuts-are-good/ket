package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{}
	chats    = make(map[string]*Chat)
)

type Chat struct {
	name    string
	clients map[*websocket.Conn]bool
}

func (c *Chat) broadcast(message []byte) {
	for client := range c.clients {
		err := client.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Printf("Error broadcasting message to client: %v", err)
			client.Close()
			delete(c.clients, client)
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to websocket: %v", err)
		return
	}

	chatName := r.URL.Query().Get("chatname")
	if chatName == "" {
		log.Println("Chat name is required")
		conn.Close()
		return
	}

	chat, ok := chats[chatName]
	if !ok {
		chat = &Chat{
			name:    chatName,
			clients: make(map[*websocket.Conn]bool),
		}
		chats[chatName] = chat
	}

	chat.clients[conn] = true

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			delete(chat.clients, conn)
			conn.Close()
			break
		}

		username := getUsername(conn)
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		formattedMessage := fmt.Sprintf("%s(%s): %s", username, timestamp, string(message))

		chat.broadcast([]byte(formattedMessage))
	}
}

func getUsername(conn *websocket.Conn) string {
	return conn.RemoteAddr().String()
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)

	fmt.Println("Server started on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

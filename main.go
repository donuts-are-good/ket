package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{}
	chats    = make(map[string]*Chat)
	config   *Config
)

type Config struct {
	Port         int      `json:"port"`
	ChatServer   string   `json:"chat_server"`
	URL          string   `json:"url"`
	DefaultRooms []string `json:"default_rooms"`
	SocketPath   string   `json:"socket_path"`
	WebPath      string   `json:"web_path"`
}

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

func (c *Chat) sendMOTD(conn *websocket.Conn) {
	motd, err := getMOTD(c.name)
	if err != nil {
		log.Printf("Error getting MOTD: %v", err)
		return
	}

	err = conn.WriteMessage(websocket.TextMessage, []byte(motd))
	if err != nil {
		log.Printf("Error sending MOTD to client: %v", err)
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

	// Send MOTD to new user
	if len(chat.clients) == 1 {
		chat.sendMOTD(conn)
	}

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

func loadConfig() (*Config, error) {
	file, err := os.Open("config.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func getMOTD(chatName string) (string, error) {
	motdPath := filepath.Join(config.WebPath, "motd", chatName+".motd.txt")
	if _, err := os.Stat(motdPath); os.IsNotExist(err) {
		motdPath = filepath.Join(config.WebPath, "motd", "default.motd.txt")
	}

	motdBytes, err := os.ReadFile(motdPath)
	if err != nil {
		return "", err
	}

	return string(motdBytes), nil
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(config.WebPath, "index.html"))
}

func main() {
	var err error
	config, err = loadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	http.HandleFunc(config.SocketPath, handleWebSocket)
	http.HandleFunc("/", serveIndex)

	addr := fmt.Sprintf(":%d", config.Port)
	fmt.Printf("Server started on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

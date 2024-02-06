package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	azAZ09   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	az09     = "abcdefghijklmnopqrstuvwxyz0123456789"
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	chats  = make(map[string]*Chat)
	config *Config
	users  = make(map[*websocket.Conn]string)
)

type Config struct {
	Port         int      `json:"port"`
	ChatServer   string   `json:"chat_server"`
	URL          string   `json:"url"`
	DefaultRooms []string `json:"default_rooms"`
	SocketPath   string   `json:"socket_path"`
	WebPath      string   `json:"web_path"`
	MotdPath     string   `json:"motd_path"`
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
			delete(users, client)
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
	chat.userJoined(conn)

	chat.sendMOTD(conn)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			chat.userLeft(conn)
			break
		}

		handleMessage(conn, chat, message)
	}
}

func handleMessage(conn *websocket.Conn, chat *Chat, message []byte) {
	if strings.HasPrefix(string(message), "/user ") || strings.HasPrefix(string(message), "/username ") || strings.HasPrefix(string(message), "/name ") {
		handleUsernameChange(conn, chat, message)
	} else if strings.HasPrefix(string(message), "/users") {
		handleUserList(conn, chat)
	} else if strings.HasPrefix(string(message), "/help") {
		handleHelp(conn)
	} else {
		handleChatMessage(conn, chat, message)
	}
}

func handleHelp(conn *websocket.Conn) {
	helpMessage := `Available commands:
- /user <new_username>: Change your username
- /join <room_name>: Join a new room
- /users: List all users in the current room
- /help: Show this help screen`
	conn.WriteMessage(websocket.TextMessage, []byte(helpMessage))
}

func handleUserList(conn *websocket.Conn, chat *Chat) {
	roomName := chat.name
	userList := getUsersInRoom(roomName)
	message := fmt.Sprintf("%d users in #%s\n%s", len(userList), roomName, strings.Join(userList, ", "))
	conn.WriteMessage(websocket.TextMessage, []byte(message))
}

func getUsersInRoom(roomName string) []string {
	var userList []string
	chat, ok := chats[roomName]
	if ok {
		for conn := range chat.clients {
			username := getUsername(conn)
			userList = append(userList, username)
		}
	}
	return userList
}

func handleUsernameChange(conn *websocket.Conn, chat *Chat, message []byte) {
	newUsername := ""
	if strings.HasPrefix(string(message), "/user ") {
		newUsername = strings.TrimPrefix(string(message), "/user ")
	} else if strings.HasPrefix(string(message), "/name ") {
		newUsername = strings.TrimPrefix(string(message), "/name ")
	} else if strings.HasPrefix(string(message), "/username ") {
		newUsername = strings.TrimPrefix(string(message), "/username ")
	}

	newUsername = sanitizeString(newUsername, 20, azAZ09)
	if newUsername != "" {
		if isUsernameAvailable(newUsername) {
			oldUsername := users[conn]
			users[conn] = newUsername
			formattedMessage := fmt.Sprintf("%s is now known as %s", oldUsername, newUsername)
			chat.broadcast([]byte(formattedMessage))
		} else {
			errMessage := fmt.Sprintf("Username %s is already taken", newUsername)
			conn.WriteMessage(websocket.TextMessage, []byte(errMessage))
		}
	} else {
		errMessage := "Invalid username format"
		conn.WriteMessage(websocket.TextMessage, []byte(errMessage))
	}
}

func handleChatMessage(conn *websocket.Conn, chat *Chat, message []byte) {
	username := getUsername(conn)
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	formattedMessage := fmt.Sprintf("%s(%s): %s", username, timestamp[:10], string(message))

	chat.broadcast([]byte(formattedMessage))
}

func sanitizeString(dirtystring string, maxlength int, charset string) string {
	pattern := fmt.Sprintf("[^%s]", charset)
	reg := regexp.MustCompile(pattern)

	cleanString := reg.ReplaceAllString(dirtystring, "")

	if len(cleanString) > maxlength {
		cleanString = cleanString[:maxlength]
	}

	return cleanString
}

func getUsername(conn *websocket.Conn) string {
	username := users[conn]
	if username != "" {
		return username
	}

	charset := "ABCDEF1234567890"
	b := make([]byte, 4)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}

	username = "user#" + string(b)
	users[conn] = username
	return username
}

func isUsernameAvailable(username string) bool {
	for _, u := range users {
		if u == username {
			return false
		}
	}
	return true
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

func (c *Chat) userJoined(conn *websocket.Conn) {
	username := getUsername(conn)
	formattedMessage := fmt.Sprintf("%s has joined the chat", username)
	c.broadcast([]byte(formattedMessage))
}

func (c *Chat) userLeft(conn *websocket.Conn) {
	username := getUsername(conn)
	formattedMessage := fmt.Sprintf("%s has left the chat", username)
	c.broadcast([]byte(formattedMessage))
	delete(c.clients, conn)
	conn.Close()
	delete(users, conn)
}

func getMOTD(chatName string) (string, error) {
	if chatName == "" {
		return "", fmt.Errorf("Chat name is required")
	}

	sanitizedChatName := sanitizeString(chatName, 50, az09)
	if sanitizedChatName != chatName {
		return "", fmt.Errorf("invalid characters in chat name")
	}

	motdPath := filepath.Join(config.MotdPath, sanitizedChatName+".motd.txt")
	if _, err := os.Stat(motdPath); os.IsNotExist(err) {
		motdPath = filepath.Join(config.MotdPath, "default.motd.txt")
	}

	motdBytes, err := os.ReadFile(motdPath)
	if err != nil {
		return "", fmt.Errorf("error reading MOTD file: %v", err)
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
	fmt.Printf("Server started on %s%s\n", config.URL, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

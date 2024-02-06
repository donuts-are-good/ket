# ket
chat server written in go supporting multiple rooms over websockets

### chat
clients can connect to the server using a websocket connection. the chat room name should be specified as a query parameter in the url (e.g. ws://example.com/chat?chatname=general).

once connected, clients can send messages to the server. if a message looks like "/user mycoolusername", the server will interpret the message as a new username for the client.

### server
the server uses the gorilla/websocket package to handle websocket connections. when a client connects, the server adds them to the specified chat room and sends them the message of the day (motd) for that room.

the server broadcasts all messages received from clients to all other clients in the same chat room.

### config
the server configuration is specified in a json file (config.json). the following options can be set:

- port: the port the server should run on.
- chat_server: the name of the chat server.
- url: the base url of the server.
- default_rooms: an array of default chat room names.
- socket_path: the path for websocket connections.
- web_path: the path to the web files.
- motd_path: the path to the motd files.

## license

mit license 2023 donuts-are-good, for more info see license.md

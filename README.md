# reverse-shell
a reverse shell in go

## Server

first run the server with `go run cmd/server/main.go`; this will create a server which listens for new connections.


## Client

Next run the client with `go run cmd/client/main.go`; this will connect to the server and present the server with a shell.


# Under the hood

The client creates a pseudo terminal which is sent over TCP to the server.

If the server's window is resized, it sends a resize message to the client to resize the pseudo terminal.

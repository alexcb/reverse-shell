# reverse-shell
a (semi-secure) reverse shell implemented in go



## Installation

Download the appropriate binaries from https://github.com/alexcb/reverse-shell/releases

## Server

The server must first be started, which binds to port 5143 by default. This port must be accessible from the client.

example usage:

```
./reverseshell-server-linux-amd64 --password myreallystrongpassword1
```

When a client connects, and performs an aes-encrypted symmetric two-way handshake, a shell (on the client's host) will be provided.

## Client

To provide a shell to the server (i.e. to allow the server to run any arbitrary commands on the client's host), run:

```bash
./reverseshell-client-linux-amd64 --password myreallystrongpassword1 <server-ip-address>
```

This process will then accept commands from the server and run them locally; once the server issues an exit (or ctrl-c, EOF, etc), this process will exit.

# Under the hood

The client initiates a TCP connection to the server.
Upon connection, both the client and server both send an aes-encrypted block of data using the supplied password containing a "hello world" string, followed by a random string.
This encrypted block is decoded, and verifies the random string is different.

If the handshake succeeds, the client creates a pseudo terminal and streams data over an aes-encrypted TCP stream.
This pseudo terminal includes window-resize messages, which allows applications such as `top` to run.

# TODO

The aes stream re-uses the same key for sending new blocks, which introduces a security vulnerability.

package encconn

import "net"

func New(conn net.Conn, password string) (net.Conn, error) {
	return conn, nil
}

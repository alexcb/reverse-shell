package encconn

import (
	"fmt"
	"io"
	"net"
	"os"
)

type stub struct {
	conn net.Conn

	f *os.File
}

func Stub(conn net.Conn, password string) (io.ReadWriteCloser, error) {
	ec := &stub{
		conn: conn,
	}

	f, err := os.Create("log")
	if err != nil {
		panic(err)
	}
	ec.f = f
	fmt.Fprintf(ec.f, "New stub\n")
	return ec, nil
}

func (ec *stub) Write(b []byte) (int, error) {
	fmt.Fprintf(ec.f, "write %d\n", len(b))
	n, err := ec.conn.Write(b)
	return n, err
}

func (ec *stub) Read(b []byte) (int, error) {
	n, err := ec.conn.Read(b)
	fmt.Fprintf(ec.f, "read %d\n", n)
	return n, err
}

func (ec *stub) Close() error {
	return ec.conn.Close()
}

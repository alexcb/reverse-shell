package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
	"golang.org/x/crypto/ssh/terminal"
)

type session struct {
	yaSession *yamux.Session

	ctx    context.Context
	cancel context.CancelFunc

	ttyCon     net.Conn
	resizeConn net.Conn
}

type server struct {
	session *session

	sigs chan os.Signal
}

func writeUint16PrefixedData(w io.Writer, data []byte) error {
	length := uint16(len(data))
	err := binary.Write(w, binary.LittleEndian, length)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (s *session) sendNewWindowSize(size *pty.Winsize) error {
	//fmt.Printf("new size %v\n", size)

	b, err := json.Marshal(size)
	if err != nil {
		return err
	}

	return writeUint16PrefixedData(s.resizeConn, b)
}

//func (s *session) handle2(size *pty.Winsize) error {
//	//go func() {
//	//	_, _ = io.Copy(os.Stdout, s.ttyCon)
//	//	s.cancel()
//	//}()
//	//go func() {
//	//	_, _ = io.Copy(s.ttyCon, os.Stdin)
//	//	s.cancel()
//	//}()
//
//	//<-s.ctx.Done()
//	return nil
//}

func (s *session) handle() error {
	for {
		stream, err := s.yaSession.Accept()
		if err != nil {
			panic(err)
		}

		buf := make([]byte, 1)
		stream.Read(buf)

		switch buf[0] {
		case 0x01:
			go s.handle1(stream)

		case 0x02:
			s.resizeConn = stream
		default:
			panic("unsupported stream code")
		}

	}
}

func (s *session) handle1(conn net.Conn) error {
	go func() {
		_, _ = io.Copy(os.Stdout, conn)
		s.cancel()
	}()
	go func() {
		_, _ = io.Copy(conn, os.Stdin)
		s.cancel()
	}()

	<-s.ctx.Done()
	return nil
}

//func (s *session) handle2(conn net.Conn) error {
//	for {
//		_ = <-s.sigs
//		if len(s.sigs) > 0 {
//			continue
//		}
//		size, err := pty.GetsizeFull(os.Stdin)
//		if err != nil {
//			fmt.Printf("failed to get size: %v\n", err)
//		} else {
//			fmt.Printf("size is %v\n", size)
//		}
//		if s.resizeConn != nil {
//			s.resizeConn.Write([]byte("hello"))
//		}
//	}
//}

func (s *server) handleRequest(conn net.Conn) error {
	defer conn.Close()

	yaSession, err := yamux.Server(conn, nil)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.session = &session{
		yaSession: yaSession,
		ctx:       ctx,
		cancel:    cancel,
	}
	defer func() { s.session = nil }()

	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = terminal.Restore(int(os.Stdin.Fd()), oldState) }()

	s.session.handle()

	return nil
}

func (s *server) windowResizeHandler() error {
	for {
		_ = <-s.sigs
		if len(s.sigs) > 0 {
			continue
		}
		size, err := pty.GetsizeFull(os.Stdin)
		if err != nil {
			fmt.Printf("failed to get size: %v\n", err)
		} else {
			if s.session != nil {
				s.session.sendNewWindowSize(size)
			}
		}
	}
}

// Start starts the debug server listener
func (s *server) Start() error {
	l, err := net.Listen("tcp", "127.0.0.1:5432")
	if err != nil {
		return err
	}
	defer l.Close()

	go s.windowResizeHandler()

	for {
		// Listen for an incoming connection.
		conn, err := l.Accept()
		if err != nil {
			fmt.Printf("Error accepting: %v", err.Error())
			os.Exit(1)
		}
		// Handle connections in a new goroutine.
		err = s.handleRequest(conn)
		if err != nil {
			fmt.Printf("lost connection to interactive debugger: %v\n", err)
		} else {
			fmt.Printf("interactive debugger closed\n")
		}
	}

	return nil
}

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGWINCH)

	srv := &server{
		sigs: sigs,
	}

	err := srv.Start()
	if err != nil {
		panic(err)
	}
}

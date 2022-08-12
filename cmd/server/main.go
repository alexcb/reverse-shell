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
	"path"
	"syscall"

	"github.com/alexcb/reverseshell/v2/encconn"
	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
	"github.com/jessevdk/go-flags"
	"golang.org/x/crypto/ssh/terminal"
)

type session struct {
	yaSession *yamux.Session

	ctx    context.Context
	cancel context.CancelFunc

	ttyCon     net.Conn
	resizeConn net.Conn

	server *server
}

type server struct {
	session *session

	ctx    context.Context
	cancel context.CancelFunc

	sigs     chan os.Signal
	password string
	bindAddr string
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
	b, err := json.Marshal(size)
	if err != nil {
		return err
	}
	return writeUint16PrefixedData(s.resizeConn, b)
}

func (s *session) handle() error {
	for {
		stream, err := s.yaSession.Accept()
		if err != nil {
			return err
		}

		buf := make([]byte, 1)
		stream.Read(buf)

		switch buf[0] {
		case 0x01:
			go s.handle1(stream)

		case 0x02:
			s.resizeConn = stream
			s.server.sigs <- syscall.SIGWINCH
		default:
			return fmt.Errorf("unsupported stream code %v", buf[0])
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

func (s *server) handleRequest(stream io.ReadWriteCloser) error {
	defer stream.Close()

	yaSession, err := yamux.Server(stream, nil)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.session = &session{
		yaSession: yaSession,
		ctx:       ctx,
		cancel:    cancel,
		server:    s,
	}
	defer cancel()
	defer func() { s.session = nil }()

	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = terminal.Restore(int(os.Stdin.Fd()), oldState) }()

	return s.session.handle()
}

func (s *server) windowResizeHandler() error {
	for {
		select {
		case _ = <-s.sigs:

		case <-s.ctx.Done():
			return nil
		}
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
	l, err := net.Listen("tcp", s.bindAddr)
	if err != nil {
		return err
	}
	defer l.Close()

	go s.windowResizeHandler()

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error accepting: %v", err.Error())
			continue
		}
		fmt.Fprintf(os.Stderr, "got shell from %v\n", conn)

		var stream io.ReadWriteCloser
		if v, _ := os.LookupEnv("STUB"); v == "1" {
			stream, err = encconn.Stub(conn, s.password)
			if err != nil {
				return err
			}
		} else {
			stream, err = encconn.New(conn, s.password)
			if err != nil {
				return err
			}
		}
		//fmt.Fprintf(os.Stderr, "valid encryption handshake\n")

		err = s.handleRequest(stream)
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "lost connection to shell: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "shell closed, waiting for new shell\n")
		}
	}

	return nil
}

type opts struct {
	Verbose  bool   `long:"verbose" short:"v" description:"Enable verbose logging"`
	Version  bool   `long:"version" short:"V" description:"Print version and exit"`
	Password string `long:"password" short:"p" description:"Symetric password" required:"true"`
	Bind     string `long:"bind" short:"b" default:"0.0.0.0:5143" description:"address to bind to"`
}

func main() {
	programName := "reverseshell-server"
	if len(os.Args) > 0 {
		programName = path.Base(os.Args[0])
	}

	progOpts := opts{}
	p := flags.NewNamedParser("", flags.PrintErrors|flags.PassDoubleDash|flags.PassAfterNonOption)
	_, err := p.AddGroup(fmt.Sprintf("%s [options] args", programName), "", &progOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
	args, err := p.ParseArgs(os.Args[1:])
	if err != nil {
		p.WriteHelp(os.Stderr)
		os.Exit(1)
	}
	if len(args) != 0 {
		p.WriteHelp(os.Stderr)
		os.Exit(1)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGWINCH)

	ctx, cancel := context.WithCancel(context.Background())
	srv := &server{
		password: progOpts.Password,
		bindAddr: progOpts.Bind,
		sigs:     sigs,
		ctx:      ctx,
		cancel:   cancel,
	}
	defer cancel()

	err = srv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}

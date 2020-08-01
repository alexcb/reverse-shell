package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
)

var (
	// Version is the version of the debugger
	Version string

	// ErrNoShellFound occurs when the container has no shell
	ErrNoShellFound = fmt.Errorf("no shell found")
)

func getShellPath() (string, bool) {
	for _, sh := range []string{
		"bash", "ksh", "zsh", "sh",
	} {
		if path, err := exec.LookPath(sh); err == nil {
			return path, true
		}
	}
	return "", false
}

func ReadUint16PrefixedData(r io.Reader) ([]byte, error) {
	var l uint16
	err := binary.Read(r, binary.LittleEndian, &l)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(io.LimitReader(r, int64(l)))
}

func interactiveMode(remoteConsoleAddr string) error {
	conn, err := net.Dial("tcp", remoteConsoleAddr)
	if err != nil {
		return err
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			fmt.Printf("error closing: %v\n", err)
		}
	}()

	session, err := yamux.Client(conn, nil)
	if err != nil {
		panic(err)
	}

	stream1, err := session.Open()
	if err != nil {
		panic(err)
	}
	stream1.Write([]byte{0x01})

	stream2, err := session.Open()
	if err != nil {
		panic(err)
	}
	stream2.Write([]byte{0x02})

	shellPath, ok := getShellPath()
	if !ok {
		return ErrNoShellFound
	}
	c := exec.Command(shellPath)

	// Start the command with a pty.
	ptmx, e := pty.Start(c)
	if e != nil {
		fmt.Printf("failed to start pty: %v\n", e)
		return e
	}
	// Make sure to close the pty at the end.
	defer func() { _ = ptmx.Close() }() // Best effort.

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_, _ = io.Copy(ptmx, stream1)
		cancel()
	}()
	go func() {
		_, _ = io.Copy(stream1, ptmx)
		cancel()
	}()
	go func() {
		_ = c.Wait()
		cancel()
	}()

	go func() {
		for {
			fmt.Printf("readingv\n")
			data, err := ReadUint16PrefixedData(stream2)
			if err != nil {
				panic(err)
			}

			var size pty.Winsize
			err = json.Unmarshal(data, &size)
			if err != nil {
				panic(err)
			}

			fmt.Printf("setsize: %v\n", size)
			err = pty.Setsize(ptmx, &size)
			if err != nil {
				panic(err)
			}

		}
	}()

	<-ctx.Done()

	fmt.Fprintf(os.Stderr, "exiting interactive debugger shell\n")
	return nil
}

func main() {
	remoteConsoleAddr := "127.0.0.1:5432"
	err := interactiveMode(remoteConsoleAddr)
	if err != nil {
		panic(err)
	}
}

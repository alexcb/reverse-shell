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
	"path"

	"github.com/alexcb/reverseshell/v2/encconn"
	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
	"github.com/jessevdk/go-flags"
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

func interactiveMode(remoteConsoleAddr, password string) error {
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

	var stream io.ReadWriteCloser

	if v, _ := os.LookupEnv("STUB"); v == "1" {
		stream, err = encconn.Stub(conn, password)
		if err != nil {
			return err
		}
	} else {
		stream, err = encconn.New(conn, password)
		if err != nil {
			return err
		}
	}

	session, err := yamux.Client(stream, nil)
	if err != nil {
		panic(err)
	}

	stream1, err := session.Open()
	if err != nil {
		fmt.Printf("wat?\n")
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
			data, err := ReadUint16PrefixedData(stream2)
			if err == io.EOF {
				return
			} else if err != nil {
				panic(err)
			}

			var size pty.Winsize
			err = json.Unmarshal(data, &size)
			if err != nil {
				panic(err)
			}

			//fmt.Printf("setsize: %v\n", size)
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

type opts struct {
	Verbose  bool   `long:"verbose" short:"v" description:"Enable verbose logging"`
	Version  bool   `long:"version" short:"V" description:"Print version and exit"`
	Password string `long:"password" short:"p" description:"Symetric password"`
}

func main() {

	programName := "reverseshell-client"
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
	if len(args) != 1 {
		p.WriteHelp(os.Stderr)
		os.Exit(1)
	}
	host := args[0]

	err = interactiveMode(host, progOpts.Password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}

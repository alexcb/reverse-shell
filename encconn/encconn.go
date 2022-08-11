package encconn

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/crypto/pbkdf2"
)

const magicString = "hello world"

type encConn struct {
	conn             net.Conn
	key              []byte
	sentHandshakeBlk []byte

	// encrypted bytes
	buf []byte

	// decrypted bytes
	payloadBuf []byte

	f *os.File
}

// New returns a new aes-block encoded stream
func New(conn net.Conn, password string) (io.ReadWriteCloser, error) {
	key := pbkdf2.Key([]byte(password), []byte("d76cd86b-4237-4ef2-befd-0384a64d47c7"), 100, 32, sha256.New)
	ec := &encConn{
		conn: conn,
		key:  key,
	}

	f, err := os.Create("log")
	if err != nil {
		panic(err)
	}
	ec.f = f
	fmt.Fprintf(ec.f, "New encConn\n")

	err = ec.handshake()
	if err != nil {
		return nil, err
	}
	return ec, nil
}

var errTooBig = fmt.Errorf("data is larger than a block")

func makeBlock(data, key []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	n := len(data)
	if err := binary.Write(buf, binary.LittleEndian, uint64(n)); err != nil {
		return nil, err
	}
	if _, err := buf.Write(data); err != nil {
		return nil, err
	}
	paddingN := aes.BlockSize - (buf.Len() % aes.BlockSize)
	if paddingN > 0 {
		padding := make([]byte, paddingN)
		if _, err := rand.Read(padding); err != nil {
			return nil, err
		}
		if _, err := buf.Write(padding); err != nil {
			return nil, err
		}
	}
	plaintext := buf.Bytes()

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	mode := cipher.NewCBCEncrypter(block, iv) // TODO encryptor should persist between stream calls (currently this is much closer to ECB mode)
	mode.CryptBlocks(ciphertext[aes.BlockSize:], plaintext)

	return ciphertext, nil
}

var errDecodeUnderrun = fmt.Errorf("decode underrun")

func decodeBlock(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < aes.BlockSize {
		panic("ciphertext too short")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		panic("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	// works inplace when both args are the same
	mode.CryptBlocks(ciphertext, ciphertext)

	bn := len(ciphertext)
	buf := bytes.NewReader(ciphertext)
	var n uint64
	if err = binary.Read(buf, binary.LittleEndian, &n); err != nil {
		return nil, err
	}
	if (n + 8) > uint64(bn) {
		return nil, errDecodeUnderrun // most likely the key is invalid
	}
	payload := make([]byte, n)
	if _, err = buf.Read(payload); err != nil {
		return nil, err
	}

	return payload, nil
}

var errSameHandshake = fmt.Errorf("received identical handshake; hack-attempt")
var errBadMagicString = fmt.Errorf("decryption worked, but got bad magic string")

func prefixDataWithLen(b []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	n := len(b)
	if err := binary.Write(buf, binary.LittleEndian, uint64(n)); err != nil {
		return nil, err
	}
	if _, err := buf.Write(b); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var errChunkUnderrun = fmt.Errorf("chunk underrun")

func readChunk(b []byte) ([]byte, []byte, error) {
	bn := len(b)
	if bn < 8 {
		return nil, nil, errChunkUnderrun
	}
	buf := bytes.NewReader(b)
	var n uint64
	if err := binary.Read(buf, binary.LittleEndian, &n); err != nil {
		return nil, nil, err
	}
	if (n + 8) > uint64(bn) {
		return nil, nil, errChunkUnderrun
	}
	payload := make([]byte, n)
	if _, err := buf.Read(payload); err != nil {
		return nil, nil, err
	}
	remaining := b[(n + 8):]
	return payload, remaining, nil
}

var errPasswordMismatch = fmt.Errorf("handshake password mismatch")

func (ec *encConn) handshake() error {
	handshake := fmt.Sprintf("%s%s", magicString, RandomAlphanumeric(32))
	n := len(handshake)

	_, err := ec.Write([]byte(handshake))
	if err != nil {
		return err
	}

	payload := make([]byte, n)
	_, err = ec.Read(payload)
	if err != nil {
		if err == errDecodeUnderrun {
			return errPasswordMismatch
		}
		return err
	}

	if string(payload) == handshake {
		return errSameHandshake
	}
	n = len(magicString)
	if string(payload)[:n] != magicString {
		return errBadMagicString
	}
	return nil
}

// must return int on success
func (ec *encConn) Read(b []byte) (int, error) {
	if len(ec.payloadBuf) > 0 {
		n := copy(b, ec.payloadBuf)
		ec.payloadBuf = ec.payloadBuf[n:]
		fmt.Fprintf(ec.f, "read %d\n", n)
		return n, nil
	}

	if len(ec.buf) > 0 {
		chunk, remaining, err := readChunk(ec.buf)
		if err != nil {
			if err != errChunkUnderrun {
				return 0, err
			}
			fmt.Fprintf(ec.f, "underrun1\n")
			// otherwise keep going to blocking network read call
		} else {
			ec.buf = remaining
			payload, err := decodeBlock(chunk, ec.key)
			if err != nil {
				return 0, err
			}
			ec.payloadBuf = payload
			return ec.Read(b)
		}
	}

	var chunk []byte
	for {
		buf := make([]byte, 1024)
		n, err := ec.conn.Read(buf)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			panic("got empty payload")
		}
		fmt.Fprintf(ec.f, "wire-read %d\n", n)

		ec.buf = append(ec.buf, buf[:n]...)

		var remaining []byte
		chunk, remaining, err = readChunk(ec.buf)
		if err != nil {
			if err == errChunkUnderrun {
				fmt.Fprintf(ec.f, "underrun2\n")
				continue
			}
			return 0, err
		}
		ec.buf = remaining
		break
	}

	payload, err := decodeBlock(chunk, ec.key)
	if err != nil {
		return 0, err
	}

	if len(payload) == 0 {
		panic("decoded, but payload was empty")
	}

	ec.payloadBuf = payload
	return ec.Read(b)
}

// only returns int on err
func (ec *encConn) Write(b []byte) (int, error) {
	fmt.Fprintf(ec.f, "Write %d\n", len(b))
	_, n, err := ec.write(b)
	return n, err
}

func (ec *encConn) write(b []byte) ([]byte, int, error) {
	if len(b) == 0 {
		panic("cant write zero bytes")
	}

	blk, err := makeBlock(b, ec.key)
	if err != nil {
		return nil, 0, err
	}

	blk, err = prefixDataWithLen(blk)
	if err != nil {
		return nil, 0, err
	}
	if len(blk) == 0 {
		panic("blk is zero")
	}

	fmt.Fprintf(ec.f, "wire-write %d\n", len(blk))
	_, err = ec.conn.Write(blk)
	if err != nil {
		return nil, 0, err
	}
	return blk, 0, nil
}

func (ec *encConn) Close() error {
	return ec.conn.Close()
}

//func (ec *encConn) LocalAddr() net.Addr {
//	return ec.conn.LocalAddr()
//}
//
//func (ec *encConn) RemoteAddr() net.Addr {
//	return ec.conn.RemoteAddr()
//}
//
//func (ec *encConn) SetDeadline(t time.Time) error {
//	return ec.conn.SetDeadline(t)
//}
//
//func (ec *encConn) SetReadDeadline(t time.Time) error {
//	return ec.conn.SetReadDeadline(t)
//}
//
//func (ec *encConn) SetWriteDeadline(t time.Time) error {
//	return ec.conn.SetWriteDeadline(t)
//}

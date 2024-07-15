package pipe_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/gaukas/io/conn"
	. "github.com/gaukas/io/pipe"
)

func TestUnixPipe(t *testing.T) {
	c1, c2, err := UnixPipe(nil)
	if err != nil {
		if c1 == nil || c2 == nil {
			t.Fatalf("pipe.UnixPipe: %v", err)
		} else { // likely due to Close() call errored
			t.Logf("ignoring pipe.UnixPipe: %v", err)
		}
	}

	var buf []byte = make([]byte, 1024)
	var recvBuf []byte = make([]byte, 1024)

	runtime.GC()
	runtime.Gosched()

	// Test c1 -> c2
	for i := 0; i < 10; i++ {
		if _, err := rand.Read(buf); err != nil {
			t.Errorf("rand.Read() failed: %v", err)
		}
		nWr, err := c1.Write(buf)
		if err != nil {
			t.Errorf("c1.Write() failed: %v", err)
		}

		if nWr != len(buf) {
			t.Errorf("c1.Write() wrote %d bytes, expected %d", nWr, len(buf))
		}

		nRd, err := c2.Read(recvBuf)
		if err != nil {
			t.Fatalf("c2.Read() failed: %v", err)
		}

		if !bytes.Equal(buf, recvBuf[:nRd]) {
			t.Fatalf("sent and received buffers are not equal")
		}
	}

	runtime.GC()
	runtime.Gosched()

	// Test c2 -> c1
	for i := 0; i < 10; i++ {
		if _, err := rand.Read(buf); err != nil {
			t.Errorf("rand.Read() failed: %v", err)
		}
		nWr, err := c2.Write(buf)
		if err != nil {
			t.Errorf("c2.Write() failed: %v", err)
		}

		if nWr != len(buf) {
			t.Errorf("c2.Write() wrote %d bytes, expected %d", nWr, len(buf))
		}

		nRd, err := c1.Read(recvBuf)
		if err != nil {
			t.Fatalf("c1.Read() failed: %v", err)
		}

		if !bytes.Equal(buf, recvBuf[:nRd]) {
			t.Fatalf("sent and received buffers are not equal")
		}
	}

	// net.UnixConn is stream-oriented, so we don't need to test when the receiver buffer
	// is shorter than the message sent by its peer.
}

func BenchmarkUnixPipe(b *testing.B) {
	b.Run("Write", benchmarkUnixPipe_Write)
	b.Run("Read", benchmarkUnixPipe_Read)
}

func benchmarkUnixPipe_Write(b *testing.B) {
	b.SetBytes(1024)

	c1, c2, err := UnixPipe(nil)
	if err != nil {
		if c1 == nil || c2 == nil {
			b.Fatalf("pipe.UnixPipe: %v", err)
		} else { // likely due to (net.Listener).Close() call errored
			b.Logf("ignoring pipe.UnixPipe: %v", err)
		}
	}

	var buf []byte = make([]byte, 1024)

	var readerWg sync.WaitGroup
	readerWg.Add(1)
	go func(conn conn.Conn) {
		defer conn.Close()
		defer readerWg.Done()

		var discard []byte = make([]byte, 1024)
		for {
			nDiscard, err := io.ReadFull(conn, discard)
			if err != nil {
				if err != io.EOF {
					b.Errorf("conn.Read() failed: %v", err)
				}
				return
			}

			if nDiscard != 1024 {
				b.Errorf("conn.Read() read %d bytes, expected 1024", nDiscard)
				return
			}
		}
	}(c2)

	b.ResetTimer()

	func(conn conn.Conn) {
		defer conn.Close()

		for i := 0; i < b.N; i++ {
			if _, err := rand.Read(buf); err != nil {
				b.Errorf("rand.Read() failed: %v", err)
			}

			n, err := conn.Write(buf)
			if err != nil {
				b.Errorf("conn.Write() failed: %v", err)
				return
			}

			if n != 1024 {
				b.Errorf("conn.Write() read %d bytes, expected 1024", n)
				return
			}
		}
	}(c1)

	readerWg.Wait()
	b.StopTimer() // stop timer only after the reader returns, to make sure all data is received by the reader
}

func benchmarkUnixPipe_Read(b *testing.B) {
	b.SetBytes(1024)

	c1, c2, err := UnixPipe(nil)
	if err != nil {
		if c1 == nil || c2 == nil {
			b.Fatalf("pipe.UnixPipe: %v", err)
		} else { // likely due to (net.Listener).Close() call errored
			b.Logf("ignoring pipe.UnixPipe: %v", err)
		}
	}

	var blockWriter *sync.Mutex = new(sync.Mutex)
	blockWriter.Lock()

	var writerWg sync.WaitGroup
	writerWg.Add(1)

	go func(conn conn.Conn, blockingLocker sync.Locker) {
		defer conn.Close()
		defer writerWg.Done()

		blockingLocker.Lock()
		defer blockingLocker.Unlock()

		var buf []byte = make([]byte, 1024)
		for {
			if _, err := rand.Read(buf); err != nil {
				b.Errorf("rand.Read() failed: %v", err)
				return
			}

			nWr, err := conn.Write(buf)
			if err != nil {
				if !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, os.ErrClosed) && !errors.Is(err, net.ErrClosed) {
					b.Errorf("conn.Write() failed: %v", err)
				}
				return
			}

			if nWr != 1024 {
				b.Errorf("conn.Write() written %d bytes, expected 1024", nWr)
				return
			}
		}
	}(c2, blockWriter)

	b.ResetTimer()

	func(conn conn.Conn, senderToClose io.Closer, unblockLocker sync.Locker) {
		defer conn.Close()
		defer senderToClose.Close()

		unblockLocker.Unlock()
		var discard []byte = make([]byte, 1024)

		for i := 0; i < b.N; i++ {
			n, err := conn.Read(discard)
			if err != nil {
				b.Errorf("conn.Read() failed: %v", err)
				return
			}

			if n != 1024 {
				b.Errorf("conn.Read() read %d bytes, expected 1024", n)
				return
			}
		}
	}(c1, c2, blockWriter)

	b.StopTimer()

	writerWg.Wait()
}

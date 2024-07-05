package connpair_test

import (
	"bytes"
	"io"
	"runtime"
	"sync"
	"testing"

	"crypto/rand"

	"github.com/gaukas/io/conn"
	. "github.com/gaukas/io/connpair"
)

func TestChannelConnPair(t *testing.T) {
	c1, c2 := ChannelConnPair()

	var buf []byte = make([]byte, 1024)
	var recvBuf []byte = make([]byte, 1024)

	runtime.GC()
	runtime.Gosched()

	// Test c1 -> c2
	for i := 0; i < 10; i++ {
		go func() {
			rand.Read(buf)
			nWr, err := c1.Write(buf)
			if err != nil {
				t.Errorf("c1.Write() failed: %v", err)
			}

			if nWr != len(buf) {
				t.Errorf("c1.Write() wrote %d bytes, expected %d", nWr, len(buf))
			}
		}()

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
		go func() {
			rand.Read(buf)
			nWr, err := c2.Write(buf)
			if err != nil {
				t.Errorf("c2.Write() failed: %v", err)
			}

			if nWr != len(buf) {
				t.Errorf("c2.Write() wrote %d bytes, expected %d", nWr, len(buf))
			}
		}()

		nRd, err := c1.Read(recvBuf)
		if err != nil {
			t.Fatalf("c1.Read() failed: %v", err)
		}

		if !bytes.Equal(buf, recvBuf[:nRd]) {
			t.Fatalf("sent and received buffers are not equal")
		}
	}

	runtime.GC()
	runtime.Gosched()

	// ChannelConn is datagram-oriented instead of stream-oriented, but guarantees
	// integrity of the data even if read buffer is shorter than the received message.
	// In this case, the message will be truncated and the next read will return the
	// remaining part of the message.
	var bigBuf []byte = make([]byte, 16*1024) // 16 KiB
	go func() {
		rand.Read(bigBuf)
		nWr, err := c1.Write(bigBuf)
		if err != nil {
			t.Errorf("c1.Write() failed: %v", err)
		}

		if nWr != len(bigBuf) {
			t.Errorf("c1.Write() wrote %d bytes, expected %d", nWr, len(bigBuf))
		}
	}()

	var bigRecvBuf []byte = make([]byte, 32*1024) // 16 KiB
	var nRdTotal int
	for {
		var smallRecvBuf []byte = make([]byte, 1024) // 1024 bytes
		nRd, err := c2.Read(smallRecvBuf)
		if err != nil {
			t.Fatalf("c2.Read() failed: %v", err)
		}

		copy(bigRecvBuf[nRdTotal:], smallRecvBuf[:nRd])
		nRdTotal += nRd

		if nRdTotal >= len(bigBuf) {
			break
		}
	}

	if nRdTotal != len(bigBuf) {
		t.Fatalf("c2.Read() read %d bytes, expected %d", nRdTotal, len(bigBuf))
	}

	if !bytes.Equal(bigBuf, bigRecvBuf[:nRdTotal]) {
		t.Fatalf("sent and received buffers are not equal")
	}
}

func BenchmarkChannelConnPair(b *testing.B) {
	b.Run("Write", benchmarkChannelConnPair_Write)
	b.Run("Read", benchmarkChannelConnPair_Read)
}

func benchmarkChannelConnPair_Write(b *testing.B) {
	b.SetBytes(1024)

	c1, c2 := ChannelConnPair()

	var buf []byte = make([]byte, 1024)

	var readerWg sync.WaitGroup
	readerWg.Add(1)
	go func(conn conn.Conn) {
		defer conn.Close()
		defer readerWg.Done()

		var discard []byte = make([]byte, 2048)
		for {
			nDiscard, err := conn.Read(discard)
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

func benchmarkChannelConnPair_Read(b *testing.B) {
	b.SetBytes(1024)

	c1, c2 := ChannelConnPair()

	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func(conn conn.Conn) {
		defer conn.Close()
		defer writerWg.Done()

		var buf []byte = make([]byte, 1024)
		for {
			if _, err := rand.Read(buf); err != nil {
				b.Errorf("rand.Read() failed: %v", err)
				return
			}

			nWr, err := conn.Write(buf)
			if err != nil {
				if err != io.ErrClosedPipe {
					b.Errorf("conn.Write() failed: %v", err)
				}
				return
			}

			if nWr != 1024 {
				b.Errorf("conn.Write() written %d bytes, expected 1024", nWr)
				return
			}
		}
	}(c2)

	b.ResetTimer()

	func(conn conn.Conn, senderToClose io.Closer) {
		defer conn.Close()
		defer senderToClose.Close()

		var discard []byte = make([]byte, 2048)

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
	}(c1, c2)

	b.StopTimer()

	writerWg.Wait()
}

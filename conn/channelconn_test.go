package conn_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	. "github.com/gaukas/io/conn"
)

var (
	rx    chan []byte
	tx    chan []byte
	cconn *ChannelConn
)

func TestChannelConn(t *testing.T) {
	t.Run("Conn", testChannelConn_Conn)
	t.Run("NetworkConn", testChannelConn_NetworkConn)
	t.Run("DeadlineConn", testChannelConn_DeadlineConn)
	t.Run("NonblockingConn", testChannelConn_NonblockingConn)
	t.Run("PollConn", testChannelConn_PollConn)
}

func testChannelConn_Conn(t *testing.T) {
	rx = make(chan []byte)
	defer close(rx)

	tx = make(chan []byte)
	cconn = NewChannelConn(rx, tx)

	t.Run("Read", testChannelConn_Conn_Read)
	t.Run("Write", testChannelConn_Conn_Write)
	t.Run("Close", testChannelConn_Conn_Close)

	if err := cconn.Close(); err != nil && err != io.ErrClosedPipe {
		t.Fatalf("cconn.Close() failed: %v", err)
	}
}

func testChannelConn_Conn_Read(t *testing.T) {
	var sendMsg []byte = make([]byte, 32)
	var recvBuf []byte = make([]byte, 64)

	rand.Read(sendMsg)

	// goroutine writing to rx
	go func() {
		sendMsgCopy := make([]byte, len(sendMsg))
		copy(sendMsgCopy, sendMsg)
		rx <- sendMsgCopy
	}()

	// read from cconn
	n, err := cconn.Read(recvBuf)
	if err != nil {
		t.Fatalf("cconn.Read() failed: %v", err)
	}

	if n != len(sendMsg) {
		t.Fatalf("cconn.Read() read %d bytes, expected %d", n, len(sendMsg))
	}

	if !bytes.Equal(sendMsg, recvBuf[:n]) {
		t.Fatalf("sent and received buffers are not equal")
	}
}

func testChannelConn_Conn_Write(t *testing.T) {
	var sendBuf []byte = make([]byte, 32)
	var recvMsg []byte = make([]byte, 64)

	rand.Read(sendBuf)

	// goroutine writing to cconn
	go func() {
		n, err := cconn.Write(sendBuf)
		if err != nil {
			t.Errorf("cconn.Write() failed: %v", err)
		}

		if n != len(sendBuf) {
			t.Errorf("cconn.Write() wrote %d bytes, expected %d", n, len(sendBuf))
		}
	}()

	// read from tx
	recvMsg = <-tx

	if len(recvMsg) != len(sendBuf) {
		t.Fatalf("tx received %d bytes, expected %d", len(recvMsg), len(sendBuf))
	}

	if !bytes.Equal(sendBuf, recvMsg) {
		t.Fatalf("sent and received buffers are not equal")
	}
}

func testChannelConn_Conn_Close(t *testing.T) {
	if err := cconn.Close(); err != nil {
		t.Fatalf("cconn.Close() failed: %v", err)
	}

	// test double close
	if err := cconn.Close(); err != io.ErrClosedPipe {
		t.Fatalf("expected %v, got %v", io.ErrClosedPipe, err)
	}

	// tx must be closed
	if b, ok := <-tx; ok || b != nil {
		t.Fatalf("tx is not closed")
	}

	// must not be able to read
	if _, err := cconn.Read(nil); err != io.ErrClosedPipe {
		t.Fatalf("expected %v, got %v", io.ErrClosedPipe, err)
	}

	// must not be able to write
	if _, err := cconn.Write(nil); err != io.ErrClosedPipe {
		t.Fatalf("expected %v, got %v", io.ErrClosedPipe, err)
	}
}

func testChannelConn_NetworkConn(t *testing.T) {
	t.Skipf("ChannelConn does not implement NetworkConn")
}

func testChannelConn_DeadlineConn(t *testing.T) {
	t.Run("SetDeadline", testChannelConn_DeadlineConn_SetDeadline)
	t.Run("SetReadDeadline", testChannelConn_DeadlineConn_SetReadDeadline)
	t.Run("SetWriteDeadline", testChannelConn_DeadlineConn_SetWriteDeadline)
}

func testChannelConn_DeadlineConn_SetDeadline(t *testing.T) {
	cconn := NewChannelConn(nil, nil)
	if err := cconn.SetDeadline(time.Time{}); err != os.ErrNoDeadline {
		t.Fatalf("expected %v, got %v", os.ErrNoDeadline, err)
	}
}

func testChannelConn_DeadlineConn_SetReadDeadline(t *testing.T) {
	cconn := NewChannelConn(nil, nil)
	if err := cconn.SetReadDeadline(time.Time{}); err != os.ErrNoDeadline {
		t.Fatalf("expected %v, got %v", os.ErrNoDeadline, err)
	}
}

func testChannelConn_DeadlineConn_SetWriteDeadline(t *testing.T) {
	cconn := NewChannelConn(nil, nil)
	if err := cconn.SetWriteDeadline(time.Time{}); err != os.ErrNoDeadline {
		t.Fatalf("expected %v, got %v", os.ErrNoDeadline, err)
	}
}

func testChannelConn_NonblockingConn(t *testing.T) {
	t.Run("IsNonblock", testChannelConn_NonblockingConn_IsNonblock)
	t.Run("SetNonblock", testChannelConn_NonblockingConn_SetNonblock)
}

func testChannelConn_NonblockingConn_IsNonblock(t *testing.T) {
	if cconn.IsNonblock() {
		t.Fatalf("the default non-blocking mode is expected to be false, got true")
	}

	if !cconn.SetNonblock(true) {
		t.Fatalf("failed to set non-blocking mode to true")
	}

	if !cconn.IsNonblock() {
		t.Fatalf("expected non-blocking mode to be true, got false")
	}

	if !cconn.SetNonblock(false) {
		t.Fatalf("failed to set non-blocking mode to false")
	}

	if cconn.IsNonblock() {
		t.Fatalf("expected non-blocking mode to be false, got true")
	}

	cconn.SetNonblock(true) // for the next unit test
}

func testChannelConn_NonblockingConn_SetNonblock(t *testing.T) {
	t.Run("Buffered", testChannelConn_NonblockingConn_SetNonblock_Buffered)
	t.Run("Unbuffered", testChannelConn_NonblockingConn_SetNonblock_Unbuffered)
}

func testChannelConn_NonblockingConn_SetNonblock_Buffered(t *testing.T) {
	rx = make(chan []byte, 1)
	defer close(rx)

	tx = make(chan []byte, 1)
	cconn = NewChannelConn(rx, tx)
	cconn.SetNonblock(true)

	t.Run("Read", testChannelConn_NonblockingConn_SetNonblock_Buffered_Read)
	t.Run("Write", testChannelConn_NonblockingConn_SetNonblock_Buffered_Write)

	if err := cconn.Close(); err != nil && err != io.ErrClosedPipe {
		t.Fatalf("cconn.Close() failed: %v", err)
	}
}

func testChannelConn_NonblockingConn_SetNonblock_Buffered_Read(t *testing.T) {
	var recvBuf []byte = make([]byte, 64)

	// read without any pending message should yield syscall.EAGAIN
	if n, err := cconn.Read(recvBuf); err != syscall.EAGAIN {
		t.Fatalf("expected %v, got %v", syscall.EAGAIN, err)
	} else if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}

	// write to rx
	var sendMsg []byte = make([]byte, 32)
	rand.Read(sendMsg)

	var sendMsgCopy []byte = make([]byte, len(sendMsg))
	copy(sendMsgCopy, sendMsg)
	rx <- sendMsgCopy

	// read from cconn
	if n, err := cconn.Read(recvBuf); err != nil {
		t.Fatalf("cconn.Read() failed: %v", err)
	} else if n != len(sendMsg) {
		t.Fatalf("cconn.Read() read %d bytes, expected %d", n, len(sendMsg))
	} else if !bytes.Equal(sendMsg, recvBuf[:n]) {
		t.Fatalf("sent and received buffers are not equal")
	}

	// read again, should yield syscall.EAGAIN since there is no pending message
	if n, err := cconn.Read(recvBuf); err != syscall.EAGAIN {
		t.Fatalf("expected %v, got %v", syscall.EAGAIN, err)
	} else if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}
}

func testChannelConn_NonblockingConn_SetNonblock_Buffered_Write(t *testing.T) {
	var sendBuf []byte = make([]byte, 32)
	rand.Read(sendBuf)

	// write without full buffer should succeed
	if n, err := cconn.Write(sendBuf); err != nil {
		t.Fatalf("cconn.Write() failed: %v", err)
	} else if n != len(sendBuf) {
		t.Fatalf("cconn.Write() wrote %d bytes, expected %d", n, len(sendBuf))
	}

	// write with full buffer should yield syscall.EAGAIN
	if n, err := cconn.Write(sendBuf); err != syscall.EAGAIN {
		t.Fatalf("expected %v, got %v", syscall.EAGAIN, err)
	} else if n != 0 {
		t.Fatalf("expected 0 bytes written, got %d", n)
	}

	// read from tx
	recvMsg := <-tx
	if len(recvMsg) != len(sendBuf) {
		t.Fatalf("tx received %d bytes, expected %d", len(recvMsg), len(sendBuf))
	}

	if !bytes.Equal(sendBuf, recvMsg) {
		t.Fatalf("sent and received buffers are not equal")
	}

	// write again, should succeed
	if n, err := cconn.Write(sendBuf); err != nil {
		t.Fatalf("cconn.Write() failed: %v", err)
	} else if n != len(sendBuf) {
		t.Fatalf("cconn.Write() wrote %d bytes, expected %d", n, len(sendBuf))
	}
}

func testChannelConn_NonblockingConn_SetNonblock_Unbuffered(t *testing.T) {
	rx = make(chan []byte)
	defer close(rx)

	tx = make(chan []byte)
	cconn = NewChannelConn(rx, tx)
	cconn.SetNonblock(true)

	t.Run("Read", testChannelConn_NonblockingConn_SetNonblock_Unbuffered_Read)
	t.Run("Write", testChannelConn_NonblockingConn_SetNonblock_Unbuffered_Write)

	if err := cconn.Close(); err != nil && err != io.ErrClosedPipe {
		t.Fatalf("cconn.Close() failed: %v", err)
	}
}

func testChannelConn_NonblockingConn_SetNonblock_Unbuffered_Read(t *testing.T) {
	var recvBuf []byte = make([]byte, 64)

	// read without any pending write to rx should yield syscall.EAGAIN
	if n, err := cconn.Read(recvBuf); err != syscall.EAGAIN {
		t.Fatalf("expected %v, got %v", syscall.EAGAIN, err)
	} else if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}

	// use a goroutine to write to rx
	var sendMsg []byte = make([]byte, 32)
	rand.Read(sendMsg)

	var sendMsgCopy []byte = make([]byte, len(sendMsg))
	copy(sendMsgCopy, sendMsg)
	go func() { rx <- sendMsgCopy }()
	timeGoroutine := time.Now()
	runtime.Gosched()

	// read from cconn, retry within a short period since we don't know exactly when the goroutine will execute
	var n int
	var err error
	n, err = cconn.Read(recvBuf)
	for n == 0 && err == syscall.EAGAIN && time.Since(timeGoroutine) < 10*time.Millisecond {
		n, err = cconn.Read(recvBuf)
		runtime.Gosched()
	}

	if err != nil {
		t.Fatalf("cconn.Read() failed: %v", err)
	} else if n != len(sendMsg) {
		t.Fatalf("cconn.Read() read %d bytes, expected %d", n, len(sendMsg))
	} else if !bytes.Equal(sendMsg, recvBuf[:n]) {
		t.Fatalf("sent and received buffers are not equal")
	}

	// read again, should yield syscall.EAGAIN since there is no pending write to rx
	if n, err := cconn.Read(recvBuf); err != syscall.EAGAIN {
		t.Fatalf("expected %v, got %v", syscall.EAGAIN, err)
	} else if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}
}

func testChannelConn_NonblockingConn_SetNonblock_Unbuffered_Write(t *testing.T) {
	var sendBuf []byte = make([]byte, 32)
	rand.Read(sendBuf)

	// write without a pending reader on tx should yield syscall.EAGAIN
	if n, err := cconn.Write(sendBuf); err != syscall.EAGAIN {
		t.Fatalf("expected %v, got %v", syscall.EAGAIN, err)
	} else if n != 0 {
		t.Fatalf("expected 0 bytes written, got %d", n)
	}

	// start reading from tx
	var recvMsg []byte
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		recvMsg = <-tx
	}()
	timeGoroutine := time.Now()
	runtime.Gosched()

	// write to cconn, retry within a short period since we don't know exactly when the goroutine will execute
	var n int
	var err error
	n, err = cconn.Write(sendBuf)
	for n == 0 && err == syscall.EAGAIN && time.Since(timeGoroutine) < 10*time.Millisecond {
		n, err = cconn.Write(sendBuf)
		runtime.Gosched()
	}

	if err != nil {
		t.Fatalf("cconn.Write() failed: %v", err)
	} else if n != len(sendBuf) {
		t.Fatalf("cconn.Write() wrote %d bytes, expected %d", n, len(sendBuf))
	}

	wg.Wait()
	if !bytes.Equal(sendBuf, recvMsg) {
		t.Fatalf("sent and received buffers are not equal")
	}

	// write again, should yield syscall.EAGAIN since there is a pending read from tx
	if n, err := cconn.Write(sendBuf); err != syscall.EAGAIN {
		t.Fatalf("expected %v, got %v", syscall.EAGAIN, err)
	} else if n != 0 {
		t.Fatalf("expected 0 bytes written, got %d", n)
	}
}

func testChannelConn_PollConn(t *testing.T) {
	t.Run("Buffered", testChannelConn_PollConn_Buffered)
}

func testChannelConn_PollConn_Buffered(t *testing.T) {
	rx = make(chan []byte, 2)
	defer close(rx)

	tx = make(chan []byte, 2)
	cconn = NewChannelConn(rx, tx)
	cconn.SetNonblock(true)

	t.Run("PollR", testChannelConn_PollConn_Buffered_PollR)
	t.Run("PollW", testChannelConn_PollConn_Buffered_PollW)
	t.Run("PollRW", testChannelConn_PollConn_Buffered_PollRW)

	if err := cconn.Close(); err != nil && err != io.ErrClosedPipe {
		t.Fatalf("cconn.Close() failed: %v", err)
	}
}

func testChannelConn_PollConn_Buffered_PollR(t *testing.T) {
	// when buffer is empty, PollR should return false
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	if ok, err := cconn.PollR(ctx); ok {
		t.Fatalf("expected false, got true")
	} else if err != context.DeadlineExceeded {
		t.Fatalf("expected %v, got %v", context.DeadlineExceeded, err)
	}

	// write to rx
	var sendMsg []byte = make([]byte, 32)
	rand.Read(sendMsg)

	var sendMsgCopy []byte = make([]byte, len(sendMsg))
	copy(sendMsgCopy, sendMsg)
	rx <- sendMsgCopy

	// PollR should return true
	ctx, cancel2 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel2()

	if ok, err := cconn.PollR(ctx); !ok {
		t.Fatalf("expected true, got false")
	} else if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// PollR again, should return true since we did not read from cconn
	ctx, cancel3 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel3()

	if ok, err := cconn.PollR(ctx); !ok {
		t.Fatalf("expected true, got false")
	} else if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// read from cconn
	var recvBuf []byte = make([]byte, 64)
	if n, err := cconn.Read(recvBuf); err != nil {
		t.Fatalf("cconn.Read() failed: %v", err)
	} else if n != len(sendMsg) {
		t.Fatalf("cconn.Read() read %d bytes, expected %d", n, len(sendMsg))
	} else if !bytes.Equal(sendMsg, recvBuf[:n]) {
		t.Fatalf("sent and received buffers are not equal")
	}

	// PollR should return false since there is no pending message
	ctx, cancel4 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel4()

	if ok, err := cconn.PollR(ctx); ok {
		t.Fatalf("expected false, got true")
	} else if err != context.DeadlineExceeded {
		t.Fatalf("expected %v, got %v", context.DeadlineExceeded, err)
	}
}

func testChannelConn_PollConn_Buffered_PollW(t *testing.T) {
	// when buffer is not full, PollW should return true
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	if ok, err := cconn.PollW(ctx); !ok {
		t.Fatalf("expected true, got false")
	} else if err != nil {
		t.Fatalf("expected nil, got %v", err)
	} else if len(tx) != 0 || cap(tx) != 2 {
		t.Fatalf("expected 0 out of 2 pending message, got %d out of %d", len(tx), cap(tx))
	}

	// write to cconn
	var sendBuf []byte = make([]byte, 32)
	rand.Read(sendBuf)

	if n, err := cconn.Write(sendBuf); err != nil {
		t.Fatalf("cconn.Write() failed: %v", err)
	} else if n != len(sendBuf) {
		t.Fatalf("cconn.Write() wrote %d bytes, expected %d", n, len(sendBuf))
	} else if len(tx) != 1 || cap(tx) != 2 {
		t.Fatalf("expected 1 out of 2 pending message, got %d out of %d", len(tx), cap(tx))
	}

	// PollW should still return true
	ctx, cancel2 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel2()

	if ok, err := cconn.PollW(ctx); !ok {
		t.Fatalf("expected true, got false")
	} else if err != nil {
		t.Fatalf("expected nil, got %v", err)
	} else if len(tx) != 1 || cap(tx) != 2 {
		t.Fatalf("expected 1 out of 2 pending message, got %d out of %d", len(tx), cap(tx))
	}

	// PollW again, should return true since we did not write to cconn
	ctx, cancel3 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel3()

	if ok, err := cconn.PollW(ctx); !ok {
		t.Fatalf("expected true, got false")
	} else if err != nil {
		t.Fatalf("expected nil, got %v", err)
	} else if len(tx) != 1 || cap(tx) != 2 {
		t.Fatalf("expected 1 out of 2 pending message, got %d out of %d", len(tx), cap(tx))
	}

	// write another message to cconn
	var sendBuf2 []byte = make([]byte, 32)
	rand.Read(sendBuf2)

	if n, err := cconn.Write(sendBuf2); err != nil {
		t.Fatalf("cconn.Write() failed: %v", err)
	} else if n != len(sendBuf2) {
		t.Fatalf("cconn.Write() wrote %d bytes, expected %d", n, len(sendBuf2))
	} else if len(tx) != 2 || cap(tx) != 2 {
		t.Fatalf("expected 2 out of 2 pending message, got %d out of %d", len(tx), cap(tx))
	}

	// PollW should return false since the buffer is full
	ctx, cancel4 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel4()

	if ok, err := cconn.PollW(ctx); ok {
		t.Fatalf("expected false, got true")
	} else if err != context.DeadlineExceeded {
		t.Fatalf("expected %v, got %v", context.DeadlineExceeded, err)
	} else if len(tx) != 2 || cap(tx) != 2 {
		t.Fatalf("expected 2 out of 2 pending message, got %d out of %d", len(tx), cap(tx))
	}

	// read 1 message from tx
	var recvMsg []byte = <-tx
	if !bytes.Equal(sendBuf, recvMsg) {
		t.Fatalf("sent and received buffers are not equal")
	}

	// PollW should return true since we read 1 message from tx
	ctx, cancel5 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel5()

	if ok, err := cconn.PollW(ctx); !ok {
		t.Fatalf("expected true, got false")
	} else if err != nil {
		t.Fatalf("expected nil, got %v", err)
	} else if len(tx) != 1 || cap(tx) != 2 {
		t.Fatalf("expected 1 out of 2 pending message, got %d out of %d", len(tx), cap(tx))
	}

	// read the other message from tx
	<-tx
	if len(tx) != 0 || cap(tx) != 2 {
		t.Fatalf("expected 0 out of 2 pending message, got %d out of %d", len(tx), cap(tx))
	}
}

func testChannelConn_PollConn_Buffered_PollRW(t *testing.T) {
	// when both buffer is empty, PollRW should return true since we can write to tx
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	if ok, err := cconn.PollRW(ctx); !ok {
		t.Fatalf("expected true, got false")
	} else if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// fill tx buffer
	tx <- make([]byte, 32)
	tx <- make([]byte, 32)

	// PollRW should return false since write buffer is full and read buffer is still empty
	ctx, cancel2 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel2()

	if ok, err := cconn.PollRW(ctx); ok {
		t.Fatalf("expected false, got true")
	} else if err != context.DeadlineExceeded {
		t.Fatalf("expected %v, got %v", context.DeadlineExceeded, err)
	}

	// fill rx buffer
	rx <- make([]byte, 32)
	rx <- make([]byte, 32)

	// PollRW should return true since both buffer is not full
	ctx, cancel3 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel3()

	if ok, err := cconn.PollRW(ctx); !ok {
		t.Fatalf("expected true, got false")
	} else if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

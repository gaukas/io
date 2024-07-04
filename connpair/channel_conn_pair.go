package connpair

import (
	"syscall"

	"github.com/gaukas/io/conn"
)

// ChannelConnPair creates a pair of interconnected [conn.ChannelConn]. Data written
// to one connection will become readable from the other.
func ChannelConnPair() (c1, c2 *conn.ChannelConn) {
	// channels as pipes
	chan1 := make(chan []byte)
	chan2 := make(chan []byte)

	return conn.NewChannelConn(chan1, chan2), conn.NewChannelConn(chan2, chan1)
}

// BufferedChannelConnPair creates a pair of interconnected [conn.ChannelConn]
// with the specified buffer size. Data written to one connection will become
// readable from the other.
func BufferedChannelConnPair(bufSize ...int) (c1, c2 *conn.ChannelConn, err error) {
	var chan1Capacity, chan2Capaticy int
	switch len(bufSize) {
	case 0: // do nothing, capacity will be 0 for both channel
	case 1:
		chan1Capacity = bufSize[0]
	case 2:
		chan1Capacity = bufSize[0]
		chan2Capaticy = bufSize[1]
	default:
		return nil, nil, syscall.EINVAL
	}

	// channels as pipes
	chan1 := make(chan []byte, chan1Capacity)
	chan2 := make(chan []byte, chan2Capaticy)

	return conn.NewChannelConn(chan1, chan2), conn.NewChannelConn(chan2, chan1), nil
}

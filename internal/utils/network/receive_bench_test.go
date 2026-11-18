package network

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"

	"github.com/flynn/noise"
)

type recordBenchmarkConn struct {
	net.Conn
	reader bytes.Reader
}

func (c *recordBenchmarkConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

// Both cipher directions are included and use monotonically increasing nonces.
// The in-memory stream isolates record allocations from unrelated goroutines.
func BenchmarkNoiseReceiveRecord(b *testing.B) {
	var key [32]byte
	key[0] = 42
	send := noise.UnsafeNewCipherState(noiseSuite, key, 0)
	raw := &recordBenchmarkConn{}
	receive := &noiseConn{Conn: raw, recv: noise.UnsafeNewCipherState(noiseSuite, key, 0)}
	payload := bytes.Repeat([]byte{0x52}, 32<<10)
	dst := make([]byte, len(payload))
	frame := make([]byte, 2, len(payload)+18)
	round := func() {
		var err error
		frame, err = send.Encrypt(frame[:2], nil, payload)
		if err != nil {
			b.Fatal(err)
		}
		binary.BigEndian.PutUint16(frame[:2], uint16(len(frame)-2))
		raw.reader.Reset(frame)
		if _, err = io.ReadFull(receive, dst); err != nil {
			b.Fatal(err)
		}
	}
	round()
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		round()
	}
}

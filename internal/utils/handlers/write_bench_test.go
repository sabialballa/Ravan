package handlers

import (
	"net"
	"testing"

	"github.com/sirupsen/logrus"
)

type relayBenchmarkConn struct{ net.Conn }

func (relayBenchmarkConn) Write(p []byte) (int, error) { return len(p), nil }
func BenchmarkRelayWriteChunk(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	dst := relayBenchmarkConn{}
	payload := make([]byte, 64<<10)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !writeAll(nil, dst, payload, logger, nil, 0, false) {
			b.Fatal("relay failed")
		}
	}
}

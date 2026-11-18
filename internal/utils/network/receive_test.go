package network

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/flynn/noise"
)

func TestNoiseReceiveRetainsPartialRecords(t *testing.T) {
	for _, pad := range []bool{false, true} {
		var key [32]byte
		key[0] = 71
		send := noise.UnsafeNewCipherState(noiseSuite, key, 0)
		var stream bytes.Buffer
		var want []byte
		for _, size := range []int{32768, 1, 8192, 3} {
			data := bytes.Repeat([]byte{byte(size % 251)}, size)
			want = append(want, data...)
			plain := data
			if pad {
				plain = append([]byte{3}, data...)
				plain = append(plain, 0, 0, 0)
			}
			encrypted, err := send.Encrypt(nil, nil, plain)
			if err != nil {
				t.Fatal(err)
			}
			var hdr [2]byte
			binary.BigEndian.PutUint16(hdr[:], uint16(len(encrypted)))
			stream.Write(hdr[:])
			stream.Write(encrypted)
		}
		raw := &recordBenchmarkConn{}
		raw.reader.Reset(stream.Bytes())
		c := &noiseConn{Conn: raw, recv: noise.UnsafeNewCipherState(noiseSuite, key, 0), pad: pad}
		got := make([]byte, len(want))
		for offset := 0; offset < len(got); {
			end := offset + 7
			if end > len(got) {
				end = len(got)
			}
			n, err := c.Read(got[offset:end])
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				t.Fatal("stalled read")
			}
			offset += n
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("partial records changed, padding=%v", pad)
		}
		if _, err := c.Read(make([]byte, 1)); err != io.EOF {
			t.Fatalf("wanted EOF, got %v", err)
		}
	}
}
func TestNoiseReceiveRejectsTamperedRecord(t *testing.T) {
	var key [32]byte
	key[0] = 9
	send := noise.UnsafeNewCipherState(noiseSuite, key, 0)
	encrypted, err := send.Encrypt(nil, nil, []byte("authenticated payload"))
	if err != nil {
		t.Fatal(err)
	}
	encrypted[len(encrypted)-1] ^= 1
	frame := make([]byte, 2)
	binary.BigEndian.PutUint16(frame, uint16(len(encrypted)))
	frame = append(frame, encrypted...)
	raw := &recordBenchmarkConn{}
	raw.reader.Reset(frame)
	c := &noiseConn{Conn: raw, recv: noise.UnsafeNewCipherState(noiseSuite, key, 0)}
	dst := bytes.Repeat([]byte{0x77}, 64)
	before := append([]byte(nil), dst...)
	n, err := c.Read(dst)
	if err == nil || n != 0 || !bytes.Equal(dst, before) {
		t.Fatal("tampered plaintext escaped authentication")
	}
}

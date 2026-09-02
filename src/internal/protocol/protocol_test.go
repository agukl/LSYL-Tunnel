package protocol

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestJSONFrameRoundTrip(t *testing.T) {
	want := OpenRequest{
		Type:            "open",
		Username:        "alice",
		ClientID:        "client-1",
		ProtocolVersion: 3,
		Target:          "127.0.0.1:3389",
	}
	var framed bytes.Buffer
	if err := WriteJSON(&framed, want); err != nil {
		t.Fatal(err)
	}

	var got OpenRequest
	if err := ReadJSON(&framed, &got, DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestWriteJSONRejectsOversizedFrame(t *testing.T) {
	value := map[string]string{"value": strings.Repeat("x", DefaultMaxHandshakeBytes)}
	if err := WriteJSON(&bytes.Buffer{}, value); err == nil || !strings.Contains(err.Error(), "handshake too large") {
		t.Fatalf("WriteJSON() error = %v, want oversized handshake", err)
	}
}

func TestReadJSONRejectsInvalidFrameLength(t *testing.T) {
	for _, test := range []struct {
		name   string
		length uint32
		want   string
	}{
		{name: "empty", length: 0, want: "empty handshake"},
		{name: "oversized", length: 17, want: "handshake too large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var framed bytes.Buffer
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], test.length)
			framed.Write(header[:])
			var target map[string]any
			err := ReadJSON(&framed, &target, 16)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReadJSON() error = %v, want %q", err, test.want)
			}
		})
	}
}

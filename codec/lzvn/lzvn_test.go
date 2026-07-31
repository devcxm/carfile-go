package lzvn

import "testing"

func TestDecodeLZVNLiteral(t *testing.T) {
	stream := append([]byte{0xe5}, []byte("hello")...)
	stream = append(stream, 0x06, 0, 0, 0, 0, 0, 0, 0)
	got, err := Decode(stream, 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeLZVNOverlappingMatch(t *testing.T) {
	stream := []byte{0x40, 0x01, 'a', 0x06, 0, 0, 0, 0, 0, 0, 0}
	got, err := Decode(stream, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "aaaa" {
		t.Fatalf("got %q", got)
	}
}

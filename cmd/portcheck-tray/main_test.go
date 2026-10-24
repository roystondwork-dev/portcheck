package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestIcoFromPNG(t *testing.T) {
	t.Parallel()
	ico := icoFromPNG(colorIcon)
	var h struct {
		Reserved, Type, Count uint16
		W, H, Colors, _       uint8
		Planes, Bits          uint16
		Size, Offset          uint32
	}
	if err := binary.Read(bytes.NewReader(ico), binary.LittleEndian, &h); err != nil {
		t.Fatal(err)
	}
	if h.Type != 1 || h.Count != 1 || h.Bits != 32 || h.Offset != 22 || int(h.Size) != len(colorIcon) {
		t.Errorf("header = %+v", h)
	}
	if !bytes.Equal(ico[22:], colorIcon) || !bytes.HasPrefix(colorIcon, []byte("\x89PNG")) {
		t.Error("the ICO doesn't carry the PNG")
	}
}

func TestCurrentUser(t *testing.T) {
	t.Parallel()
	if currentUser() == "" {
		t.Error("no current user")
	}
}

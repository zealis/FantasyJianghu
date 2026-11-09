package protocol

import "testing"

func TestWireCodecRoundtrip(t *testing.T) {
	f := Frame{Main: 3, Sub: 2, Seq: 12345, Payload: map[string]any{"x": 12.5, "name": "侠客"}}
	b, err := EncodeFrame(f)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	got, err := DecodeFrame(b)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if got.Main != f.Main || got.Sub != f.Sub || got.Seq != f.Seq {
		t.Fatalf("头部不一致: %+v vs %+v", got, f)
	}
	if got.Payload["x"] != 12.5 || got.Payload["name"] != "侠客" {
		t.Fatalf("负载不一致: %+v", got.Payload)
	}
}

func TestWireDecodeRejectsGarbage(t *testing.T) {
	if _, err := DecodeFrame([]byte("not-json")); err == nil {
		t.Fatal("非 JSON 帧应报错")
	}
}

func TestWireErrPayload(t *testing.T) {
	p := ErrPayload(2, "未鉴权")
	if p["code"] != 2 || p["msg"] != "未鉴权" {
		t.Fatalf("ErrPayload = %+v", p)
	}
}

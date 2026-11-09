// 协议帧编解码（与客户端 Protocol.frame() 同构；§5.1 帧结构）。
// Frame 置于协议层以避免 gateway ↔ router 循环依赖。
package protocol

import (
	"encoding/json"
	"fmt"
)

// Frame 为协议帧：主码 uint16 + 子码 uint16 + 序号 uint32 + JSON 负载。
type Frame struct {
	Main    uint16         `json:"main"`
	Sub     uint16         `json:"sub"`
	Seq     uint32         `json:"seq"`
	Payload map[string]any `json:"payload"`
}

func EncodeFrame(f Frame) ([]byte, error) {
	if f.Payload == nil {
		f.Payload = map[string]any{}
	}
	return json.Marshal(f)
}

func DecodeFrame(data []byte) (Frame, error) {
	var f Frame
	if err := json.Unmarshal(data, &f); err != nil {
		return Frame{}, fmt.Errorf("帧解码: %w", err)
	}
	if f.Payload == nil {
		f.Payload = map[string]any{}
	}
	return f, nil
}

// ErrPayload 为统一错误负载（M1 错误码：0 OK / 1 通用 / 2 未鉴权 / 3 参数非法 /
// 4 限流 / 5 不存在 / 6 已存在 / 7 超上限）。
func ErrPayload(code int, msg string) map[string]any {
	return map[string]any{"code": code, "msg": msg}
}

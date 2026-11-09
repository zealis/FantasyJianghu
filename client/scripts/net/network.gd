extends Node
## 网络层（前端规范 §3.4）。单 WebSocket 多路复用，主码/子码。
## 服务端权威：一切结果以服务端回包为准（开发技术规范 §1.1）。
## 连接流程：connect_and_auth() → 2/1 鉴权 → authenticated_ok(chars)。
## 心跳 15 秒（§5.2）；send_and_wait 以 seq 配对请求与回包（seq>0=回包，seq<=0=推送）。

signal authenticated_ok(chars: Array)
signal authenticated_failed(msg: String)
signal disconnected(msg: String)
signal enter_world(snapshot: Dictionary)
signal packet_received(main_code: int, sub_code: int, payload: Dictionary)

const SERVER_URL := "ws://127.0.0.1:7788/ws"
const HEARTBEAT_INTERVAL := 15.0

var offline: bool = true
var _seq: int = 0
var _socket: WebSocketPeer = WebSocketPeer.new()
var _authenticated: bool = false
var _heartbeat: Timer
var _responses: Dictionary = {}  # seq -> payload（仅 send_and_wait 等待中的序号）
var _waiting: Dictionary = {}    # seq -> true（fire-and-forget 的回包不进 _responses，避免无界增长）

func _ready() -> void:
	_heartbeat = Timer.new()
	_heartbeat.wait_time = HEARTBEAT_INTERVAL
	_heartbeat.timeout.connect(func() -> void:
		if _authenticated:
			send(1, 0, {}))
	add_child(_heartbeat)

func next_seq() -> int:
	_seq += 1
	return _seq

## 真连模式：连上后自动 2/1 鉴权；返回鉴权回包（code=0 成功）。
func connect_and_auth(url: String = SERVER_URL, token: String = SessionStore.token) -> Dictionary:
	offline = false
	_authenticated = false
	_socket = WebSocketPeer.new()
	# 重连清空序号与待配对表，避免旧连接的迟到回包与新请求串包。
	_seq = 0
	_responses.clear()
	_waiting.clear()
	_socket.connect_to_url(url)
	var deadline := Time.get_ticks_msec() + 5000
	while _socket.get_ready_state() != WebSocketPeer.STATE_OPEN:
		if Time.get_ticks_msec() > deadline or _socket.get_ready_state() == WebSocketPeer.STATE_CLOSED:
			offline = true
			authenticated_failed.emit("无法连接服务器")
			return {"code": -1, "msg": "无法连接服务器"}
		await get_tree().process_frame
	var resp := await send_and_wait(2, 1, {"token": token})
	if int(resp.get("code", -1)) == 0:
		_authenticated = true
		_heartbeat.start()
		authenticated_ok.emit(resp.get("chars", []))
	else:
		offline = true
		authenticated_failed.emit(str(resp.get("msg", "鉴权失败")))
	return resp

## 发送并等待同序号回包（超时返回 code=-1）。仅限已连接状态。
func send_and_wait(main_code: int, sub_code: int, payload: Dictionary, timeout_ms: int = 4000) -> Dictionary:
	var seq := send(main_code, sub_code, payload)
	_waiting[seq] = true
	var deadline := Time.get_ticks_msec() + timeout_ms
	while not _responses.has(seq):
		if Time.get_ticks_msec() > deadline:
			_waiting.erase(seq)
			return {"code": -1, "msg": "请求超时"}
		await get_tree().process_frame
	var resp: Dictionary = _responses[seq]
	_responses.erase(seq)
	_waiting.erase(seq)
	return resp

func send(main_code: int, sub_code: int, payload: Dictionary) -> int:
	var seq := next_seq()
	if offline:
		# 离线占位：不真正发包，不本地结算资产。
		return seq
	var frame := Protocol.frame(main_code, sub_code, seq, payload)
	_socket.send_text(JSON.stringify(frame))
	return seq

func _process(_delta: float) -> void:
	if offline:
		return
	_socket.poll()
	match _socket.get_ready_state():
		WebSocketPeer.STATE_OPEN:
			while _socket.get_available_packet_count() > 0:
				_handle_packet(_socket.get_packet().get_string_from_utf8())
		WebSocketPeer.STATE_CLOSED:
			offline = true
			_authenticated = false
			_heartbeat.stop()
			disconnected.emit("连接已断开")

func _handle_packet(raw: String) -> void:
	var data = JSON.parse_string(raw)
	if not (data is Dictionary):
		return
	var main_code: int = int(data.get("main", 0))
	var sub_code: int = int(data.get("sub", 0))
	var seq: int = int(data.get("seq", -1))
	# 负载可能为 null/非对象，先归一化再赋给 Dictionary 变量。
	var payload: Dictionary = {}
	var raw_payload = data.get("payload", {})
	if raw_payload is Dictionary:
		payload = raw_payload

	# 服务端踢下线（1/1 是推送帧，seq=0；必须早于 seq<=0 分支处理）。
	if main_code == 1 and sub_code == 1:
		offline = true
		_authenticated = false
		_heartbeat.stop()
		disconnected.emit(str(payload.get("msg", "已下线")))
		return

	# 服务端主动推送（seq<=0）：进场快照转 enter_world，其余走 packet_received。
	if seq <= 0:
		if main_code == 3 and sub_code == 1:
			enter_world.emit(payload)
		packet_received.emit(main_code, sub_code, payload)
		return

	# 请求回包：只缓存正在 send_and_wait 等待的序号。
	# 心跳/移动/确认等 fire-and-forget 的回包只分发、不滞留字典。
	if _waiting.has(seq):
		_responses[seq] = payload
	packet_received.emit(main_code, sub_code, payload)

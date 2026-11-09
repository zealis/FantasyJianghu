package account

import (
	"encoding/json"
	"net/http"
)

type httpReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

type httpResp struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data map[string]any `json:"data,omitempty"`
}

// MountHTTP 挂载 §5.3 HTTP 接口：登录/注册走 HTTPS，游戏内高频交互一律走 WS。
func (s *Service) MountHTTP(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/register", func(w http.ResponseWriter, r *http.Request) {
		var req httpReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, httpResp{Code: codeBadParams, Msg: "请求体非法"})
			return
		}
		id, code, msg := s.Register(r.Context(), req.Username, req.Password, req.Nickname)
		data := map[string]any{}
		if code == codeOK {
			data["account_id"] = id
		}
		writeJSON(w, httpResp{Code: code, Msg: msg, Data: data})
	})
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		var req httpReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, httpResp{Code: codeBadParams, Msg: "请求体非法"})
			return
		}
		token, chars, code, msg := s.Login(r.Context(), req.Username, req.Password)
		data := map[string]any{}
		if code == codeOK {
			data["token"] = token
			data["chars"] = charsList(chars)
		}
		writeJSON(w, httpResp{Code: code, Msg: msg, Data: data})
	})
}

func writeJSON(w http.ResponseWriter, resp httpResp) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

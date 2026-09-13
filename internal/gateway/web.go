package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/LouYuanbo1/go-proxy/internal/proxy"
	"github.com/LouYuanbo1/go-proxy/internal/proxyctl"
	"github.com/LouYuanbo1/go-proxy/internal/utils"
)

// Web 被动转发代理网关（策略层）
//
// 嵌入 proxy.HTTP 作为默认的 TCP 转发实现，
// 可选配 proxy.QUIC 实现 QUIC 优先转发（降级到 TCP）。
type Web struct {
	*proxy.HTTP
	quic *proxy.QUIC // 可选，为 nil 时纯 TCP
}

// NewWeb 创建被动转发代理网关实例
func NewWeb(transport *http.Transport, timeout time.Duration) *Web {
	return &Web{
		HTTP: proxy.NewHTTP(transport, timeout),
	}
}

// SetQUIC 设置 QUIC 代理实例，启用 QUIC 优先策略
func (w *Web) SetQUIC(q *proxy.QUIC) {
	w.quic = q
}

// ServeHTTP 实现 http.Handler 接口
//
// 路由规则：
//
//	GET  /                    → 管理首页
//	GET  /proxy.pac           → PAC 自动配置脚本
//	GET  /api/fetch           → 服务端抓取测试 API
//	CONNECT / GET/POST (绝对URL) → 委托给 proxy.HTTP 处理
func (w *Web) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	log.Printf("[Web] 📥 %s %s (来自 %s)", r.Method, r.URL.String(), r.RemoteAddr)

	// 路由说明：
	//   GET /               →  管理首页（r.URL.Path == "/"）
	//   CONNECT host:port   →  隧道代理（r.URL.Path == ""，落入 default）
	//   GET http://host/path → 普通代理（绝对 URL，落入 default）
	//   其他管理路径         →  各自处理
	switch r.URL.Path {
	case "/":
		w.serveHome(rw, r)

	case "/proxy.pac":
		w.servePAC(rw, r)

	case "/api/diag":
		w.serveDiag(rw, r)

	case "/api/fetch":
		if r.Method == http.MethodOptions {
			rw.Header().Set("Access-Control-Allow-Origin", "*")
			rw.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			rw.WriteHeader(http.StatusNoContent)
			return
		}
		w.serveAPIFetch(rw, r)

	default:
		// ── 策略路由：CONNECT 走 QUIC 优先 + TCP 降级 ──
		if r.Method == http.MethodConnect && w.quic != nil {
			w.handleTunnelQUIC(rw, r)
		} else {
			w.HTTP.ServeHTTP(rw, r)
		}
	}
}

// handleTunnelQUIC 处理 CONNECT 隧道，QUIC 优先 + TCP 降级
func (w *Web) handleTunnelQUIC(rw http.ResponseWriter, r *http.Request) {
	// 获取请求r的目标地址(相当于劫持原本的请求目标)
	targetAddr := r.URL.Host
	if !strings.Contains(targetAddr, ":") {
		targetAddr += ":443"
	}

	// ── 阶段 1：尝试 QUIC 连接（1 秒内必须握手完成） ──
	quicCtx, quicCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer quicCancel()

	// 尝试 QUIC 连接
	targetConn, err := w.quic.DialStream(quicCtx, targetAddr)
	if err == nil {
		log.Printf("[Web] ⚡ QUIC 隧道到 %s ✅", targetAddr)
		defer targetConn.Close()

		hijacker, ok := rw.(http.Hijacker)
		if !ok {
			http.Error(rw, "Hijacking not supported", http.StatusInternalServerError)
			return
		}

		// 作为客户端连接
		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			http.Error(rw, err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer clientConn.Close()

		// 发送成功响应
		clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

		// 代理数据流
		utils.RelayConns(r.Context(), clientConn, targetConn)
		return
	}

	// ── 阶段 2：QUIC 失败，降级到 TCP ──
	//log.Printf("[Web] ⚡ QUIC %s 不可用 (%v)，降级到 TCP", targetAddr, err)
	w.HTTP.ServeHTTP(rw, r)
}

// serveAPIFetch 服务端抓取 API
func (w *Web) serveAPIFetch(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(rw, `{"error":"缺少 url 参数"}`, http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	log.Printf("[Web:测试] ───────────────────────────────────────")
	log.Printf("[Web:测试] 🔍 服务端抓取: %s", targetURL)
	log.Printf("[Web:测试] 🕐 来源: %s", r.RemoteAddr)

	outReq, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		jsonError(rw, fmt.Sprintf("创建请求失败: %v", err), http.StatusInternalServerError)
		return
	}
	outReq.Header.Set("User-Agent", "Go-Proxy/1.0")
	outReq.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")

	resp, err := w.HTTP.Transport().RoundTrip(outReq)
	if err != nil {
		log.Printf("[Web:测试] ❌ 抓取失败: %v", err)
		log.Printf("[Web:测试] ───────────────────────────────────────")
		jsonError(rw, fmt.Sprintf("请求失败: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("[Web:测试] ✅ %s → %d %s (%d 字节)",
		targetURL, resp.StatusCode, http.StatusText(resp.StatusCode), resp.ContentLength)

	limitedReader := io.LimitReader(resp.Body, 512*1024)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		jsonError(rw, fmt.Sprintf("读取响应失败: %v", err), http.StatusInternalServerError)
		return
	}

	type headerEntry struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	var headers []headerEntry
	for k, vv := range resp.Header {
		for _, v := range vv {
			if k == "Content-Encoding" || k == "Transfer-Encoding" {
				continue
			}
			headers = append(headers, headerEntry{Key: k, Value: v})
		}
	}

	contentType := resp.Header.Get("Content-Type")
	isText := strings.Contains(contentType, "text/") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "javascript")

	displayBody := ""
	if isText {
		displayBody = string(body)
		if len(displayBody) > 500 {
			displayBody = displayBody[:500] + "..."
		}
	} else {
		displayBody = fmt.Sprintf("[二进制内容: %d 字节]", len(body))
	}

	respData := map[string]any{
		"status":      resp.StatusCode,
		"status_text": http.StatusText(resp.StatusCode),
		"body_length": len(body),
		"body":        displayBody,
		"headers":     headers,
	}

	jsonBytes, err := json.Marshal(respData)
	if err != nil {
		jsonError(rw, fmt.Sprintf("序列化失败: %v", err), http.StatusInternalServerError)
		return
	}

	rw.Write(jsonBytes)
}

// jsonError 返回 JSON 格式错误
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

// serveHome 返回管理首页
func (w *Web) serveHome(rw http.ResponseWriter, _ *http.Request) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Go-Proxy 被动转发代理</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f0f2f5;min-height:100vh;display:flex;justify-content:center;align-items:flex-start;padding:24px}
.container{background:#fff;border-radius:16px;padding:40px;box-shadow:0 8px 30px rgba(0,0,0,.08);width:100%;max-width:720px;margin-top:20px}
h1{text-align:center;color:#1a1a1a;margin-bottom:4px;font-size:22px}
.subtitle{text-align:center;color:#888;font-size:14px;margin-bottom:24px}
.card{background:#f7f8fa;border-radius:10px;padding:14px 18px;margin-bottom:10px}
.card label{display:block;font-size:11px;color:#888;margin-bottom:3px;text-transform:uppercase;letter-spacing:.5px}
.card .value{font-family:"SFMono-Regular",Consolas,"Liberation Mono",Menlo,monospace;font-size:14px;color:#333;word-break:break-all}
.card .value .tag{display:inline-block;background:#667eea;color:#fff;font-size:9px;padding:1px 7px;border-radius:4px;margin-left:8px;text-transform:uppercase}
.copy-btn{float:right;padding:1px 8px;font-size:10px;border:1px solid #ddd;border-radius:4px;background:#fff;cursor:pointer;color:#666}
.copy-btn:hover{background:#667eea;color:#fff;border-color:#667eea}
.section-title{font-size:13px;font-weight:600;color:#555;margin-bottom:10px;margin-top:4px}
.quick-test{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:14px}
.quick-test button,.quick-test a.btn{display:flex;align-items:center;gap:6px;padding:8px 14px;border:none;border-radius:8px;font-size:13px;font-weight:500;cursor:pointer;text-decoration:none;transition:all .2s;color:#fff}
.quick-test button .ico,.quick-test a.btn .ico{font-size:16px}
.quick-test .btn-baidu{background:#4e6ef2}
.quick-test .btn-baidu:hover{background:#3b5de7}
.quick-test .btn-bili{background:#fb7299}
.quick-test .btn-bili:hover{background:#f55e87}
.quick-test .btn-ip{background:#22c55e}
.quick-test .btn-ip:hover{background:#16a34a}
.quick-test .btn-headers{background:#f59e0b}
.quick-test .btn-headers:hover{background:#d97706}
.test-area{margin-bottom:14px}
.test-area form{display:flex;gap:8px}
.test-area input{flex:1;padding:10px 14px;border:2px solid #e0e0e0;border-radius:8px;font-size:14px;outline:none;transition:border-color .2s}
.test-area input:focus{border-color:#667eea}
.test-area button{padding:10px 20px;background:#667eea;color:#fff;border:none;border-radius:8px;font-size:14px;font-weight:600;cursor:pointer;transition:background .2s}
.test-area button:hover{background:#5a6fd6}
.status-dot{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:6px}
.status-dot.online{background:#22c55e}
.status-dot.offline{background:#ef4444}
.hint{font-size:13px;color:#888;margin-top:6px;line-height:1.7;padding:12px;background:#f8f9fb;border-radius:8px}
.hint strong{color:#555}
.footer{text-align:center;font-size:12px;color:#bbb;margin-top:20px}
@media(max-width:600px){.container{padding:20px}.quick-test button,.quick-test a.btn{flex:1;justify-content:center}}
</style>
</head>
<body>
<div class=container>
<h1>Go-Proxy 被动转发代理</h1>
<p class=subtitle>VPN 式透明代理 · 不修改任何流量</p>

<div class=card>
<label>代理地址 <button class=copy-btn onclick="navigator.clipboard.writeText('127.0.0.1:8090')">复制</button></label>
<div class=value>127.0.0.1:8090</div>
</div>

<div class=card>
<label>代理类型</label>
<div class=value>HTTP / HTTPS CONNECT <span class=tag>正向代理</span></div>
</div>

<div class=card>
<label>PAC 自动配置 <button class=copy-btn onclick="navigator.clipboard.writeText(window.location.origin+'/proxy.pac')">复制</button></label>
<div class=value><a href="/proxy.pac" style="color:#667eea;text-decoration:none">http://</a><span id=pac-url></span></div>
</div>

<div class=card>
<label>运行状态 <span id=status-text style="float:right;font-size:12px">检查中...</span></label>
<div class=value><span class="status-dot online" id=status-dot></span> 代理服务已启动</div>
</div>

<div class=section-title>🚀 一键测试（系统代理已配置，URL 显示真实域名）</div>
<div class=quick-test>
<a href="https://www.baidu.com" target=_blank class="btn btn-baidu"><span class=ico>B</span> 百度</a>
<a href="https://www.bilibili.com" target=_blank class="btn btn-bili"><span class=ico>B</span> 哔哩哔哩</a>
<a href="https://httpbin.org/ip" target=_blank class="btn btn-ip"><span class=ico>🖥</span> 本机 IP</a>
<a href="https://httpbin.org/headers" target=_blank class="btn btn-headers"><span class=ico>📋</span> 请求头</a>
</div>

<div class=section-title>🔗 自定义跳转</div>
<div class=test-area>
<form id=proxy-form>
<input type=text id=url-input placeholder="输入任意 URL，如 https://www.baidu.com">
<button type=submit>🚀 跳转</button>
</form>
</div>

<div class=hint>
<strong>系统代理已自动配置 ✅</strong> 程序启动时自动设置，退出时自动恢复。<br>
直接点击上方按钮或在浏览器地址栏访问任意网站即可测试。<br>
<strong>💡 如果网站打不开：</strong>重启浏览器（Chrome 缓存旧代理设置）。<br>
代理日志实时打印在终端，每个 CONNECT / HTTP 请求都会显示。
</div>

<div class=footer>Go-Proxy · 被动转发 · 日志实时输出到终端</div>
</div>

<script>
document.getElementById('proxy-form').onsubmit = function(e){
  e.preventDefault();
  var url = document.getElementById('url-input').value.trim();
  if(!url) return;
  if(!url.match(/^https?:\/\//i)) url = 'https://' + url;
  window.open(url, '_blank');
};
(function(){
  document.getElementById('pac-url').textContent = window.location.host + '/proxy.pac';
  fetch('/proxy.pac').then(function(r){
    document.getElementById('status-text').textContent = r.ok ? '在线' : '异常';
    if(!r.ok) document.getElementById('status-dot').className = 'status-dot offline';
  }).catch(function(){
    document.getElementById('status-text').textContent = '离线';
    document.getElementById('status-dot').className = 'status-dot offline';
  });
})();
</script>
</body>
</html>`
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(http.StatusOK)
	fmt.Fprint(rw, html)
}

// servePAC 返回 PAC 自动配置脚本
func (w *Web) servePAC(rw http.ResponseWriter, r *http.Request) {
	hostPort := r.Host
	if hostPort == "" {
		hostPort = "127.0.0.1:8090"
	}
	hostPort = strings.TrimPrefix(hostPort, "http://")
	hostPort = strings.TrimPrefix(hostPort, "https://")

	pac := fmt.Sprintf(`function FindProxyForURL(url, host) {
	if (shExpMatch(host, "127.0.0.*") ||
	    shExpMatch(host, "10.*") ||
	    shExpMatch(host, "172.1[6-9].*") ||
	    shExpMatch(host, "172.2[0-9].*") ||
	    shExpMatch(host, "172.3[0-1].*") ||
	    shExpMatch(host, "192.168.*") ||
	    shExpMatch(host, "localhost") ||
	    shExpMatch(host, "*.local")) {
		return "DIRECT";
	}
	return "PROXY %s; DIRECT";
}`, hostPort)

	rw.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	rw.Header().Set("Content-Length", fmt.Sprintf("%d", len(pac)))
	rw.WriteHeader(http.StatusOK)
	fmt.Fprint(rw, pac)
}

// serveDiag 返回诊断信息
func (w *Web) serveDiag(rw http.ResponseWriter, _ *http.Request) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	info := proxyctl.Current()
	data, _ := json.MarshalIndent(info, "", "  ")
	rw.Write(data)
}

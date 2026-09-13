// Package proxyctl 管理操作系统代理设置
//
// Windows：通过注册表设置 Internet Explorer / 系统代理
// 启动代理服务前保存当前设置，退出时自动恢复。
package proxyctl

// ProxySettings 保存代理配置快照，用于退出时恢复
type ProxySettings struct {
	saved bool

	// Windows 注册表 Original 值
	origEnable   uint64
	origServer   string
	origOverride string
}

// Save 保存当前系统代理设置快照
func (ps *ProxySettings) Save() {
	ps.save()
	ps.saved = true
}

// Enable 启用系统代理
//
//	addr 格式如 "127.0.0.1:8090"
func (ps *ProxySettings) Enable(addr string) error {
	return ps.enable(addr)
}

// Restore 恢复系统代理到保存时的状态
//
// 若之前未调用 Save 或已经是原始状态，则不执行任何操作。
func (ps *ProxySettings) Restore() {
	if !ps.saved {
		return
	}
	ps.restore()
}

// ProxyInfo 描述当前系统代理设置
type ProxyInfo struct {
	Enabled bool   `json:"enabled"`
	Server  string `json:"server"`
	Bypass  string `json:"bypass"`
}

// Current 读取当前系统代理设置（不修改任何内容）
func Current() ProxyInfo {
	return current()
}

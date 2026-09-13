//go:build windows

package proxyctl

import (
	"log"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const regKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

var (
	wininet                = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOptionW = wininet.NewProc("InternetSetOptionW")
)

const (
	_INTERNET_OPTION_SETTINGS_CHANGED = 39
	_INTERNET_OPTION_REFRESH          = 37
)

// notifyProxyChange 广播代理设置变更通知，让 Edge/Chrome 实时生效
//
// 对应 Clash Verge 修改注册表后调用的 WinINET API：
//
//	InternetSetOption(NULL, INTERNET_OPTION_SETTINGS_CHANGED, NULL, 0)
//	InternetSetOption(NULL, INTERNET_OPTION_REFRESH, NULL, 0)
func notifyProxyChange() {
	procInternetSetOptionW.Call(0, _INTERNET_OPTION_SETTINGS_CHANGED, 0, 0)
	procInternetSetOptionW.Call(0, _INTERNET_OPTION_REFRESH, 0, 0)
}

func (ps *ProxySettings) save() {
	k, err := registry.OpenKey(registry.CURRENT_USER, regKey, registry.QUERY_VALUE)
	if err != nil {
		log.Printf("[ProxyCtl] ⚠ 无法读取注册表: %v", err)
		return
	}
	defer k.Close()

	ps.origEnable, _, _ = k.GetIntegerValue("ProxyEnable")
	ps.origServer, _, _ = k.GetStringValue("ProxyServer")
	ps.origOverride, _, _ = k.GetStringValue("ProxyOverride")

	log.Printf("[ProxyCtl] 💾 已保存当前代理设置:"+
		" ProxyEnable=%d, ProxyServer=%q, ProxyOverride=%q",
		ps.origEnable, ps.origServer, ps.origOverride)
}

func (ps *ProxySettings) enable(addr string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, regKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyServer", addr); err != nil {
		return err
	}
	// 保留原始 bypass 列表，确保内网地址不走代理
	bypass := ps.origOverride
	if bypass == "" {
		bypass = "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*"
	}
	if err := k.SetStringValue("ProxyOverride", bypass); err != nil {
		return err
	}

	log.Printf("[ProxyCtl] ✅ 系统代理已启用 → %s", addr)
	notifyProxyChange()
	log.Printf("[ProxyCtl] 📢 已广播代理变更通知")
	return nil
}

func (ps *ProxySettings) restore() {
	k, err := registry.OpenKey(registry.CURRENT_USER, regKey, registry.SET_VALUE)
	if err != nil {
		log.Printf("[ProxyCtl] ❌ 无法打开注册表进行恢复: %v", err)
		return
	}
	defer k.Close()

	if err := k.SetDWordValue("ProxyEnable", uint32(ps.origEnable)); err != nil {
		log.Printf("[ProxyCtl] ❌ 恢复 ProxyEnable 失败: %v", err)
	}
	if err := k.SetStringValue("ProxyServer", ps.origServer); err != nil {
		log.Printf("[ProxyCtl] ❌ 恢复 ProxyServer 失败: %v", err)
	}
	if err := k.SetStringValue("ProxyOverride", ps.origOverride); err != nil {
		log.Printf("[ProxyCtl] ❌ 恢复 ProxyOverride 失败: %v", err)
	}

	status := "已禁用"
	if ps.origEnable == 1 {
		status = "已恢复"
	}
	log.Printf("[ProxyCtl] 🔄 系统代理 %s (原始状态: ProxyEnable=%d)", status, ps.origEnable)
	notifyProxyChange()
	log.Printf("[ProxyCtl] 📢 已广播代理变更通知")
}

func current() ProxyInfo {
	info := ProxyInfo{}
	k, err := registry.OpenKey(registry.CURRENT_USER, regKey, registry.QUERY_VALUE)
	if err != nil {
		return info
	}
	defer k.Close()

	enable, _, _ := k.GetIntegerValue("ProxyEnable")
	server, _, _ := k.GetStringValue("ProxyServer")
	bypass, _, _ := k.GetStringValue("ProxyOverride")

	info.Enabled = enable == 1
	info.Server = server
	info.Bypass = bypass
	return info
}

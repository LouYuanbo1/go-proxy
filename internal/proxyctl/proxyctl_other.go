//go:build !windows

package proxyctl

import "log"

func (ps *ProxySettings) save() {
	log.Printf("[ProxyCtl] ℹ 非 Windows 系统，不保存系统代理设置")
}

func (ps *ProxySettings) enable(addr string) error {
	log.Printf("[ProxyCtl] ℹ 非 Windows 系统，跳过设置系统代理 (addr=%s)", addr)
	return nil
}

func (ps *ProxySettings) restore() {
	log.Printf("[ProxyCtl] ℹ 非 Windows 系统，跳过恢复系统代理")
}

func current() ProxyInfo {
	return ProxyInfo{}
}

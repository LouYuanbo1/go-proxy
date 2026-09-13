package main

import (
	"net/http"
	"net/url"
	"testing"
)

func TestHTTPProxy(t *testing.T) {
	proxyURL, _ := url.Parse("http://127.0.0.1:8080")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("https://www.baidu.com")
	if err != nil {
		t.Fatalf("HTTP 代理请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("期望状态码 200, 得到 %d", resp.StatusCode)
	}
}

func TestSOCKS5Proxy(t *testing.T) {
	proxyURL, _ := url.Parse("socks5://127.0.0.1:1080")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("https://www.baidu.com")
	if err != nil {
		t.Fatalf("SOCKS5 代理请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("期望状态码 200, 得到 %d", resp.StatusCode)
	}
}

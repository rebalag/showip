package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		name, ua, os, browser string
	}{
		{"chrome-win", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.71 Safari/537.36", "Windows 10/11", "Chrome 120.0.6099.71"},
		{"edge-win", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.2210.61", "Windows 10/11", "Microsoft Edge 120.0.2210.61"},
		{"safari-mac", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15", "macOS 10.15.7", "Safari 17.1"},
		{"firefox-linux", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0", "Ubuntu Linux", "Firefox 121.0"},
		{"safari-iphone", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1", "iOS 17.1", "Safari 17.1"},
		{"chrome-android", "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.43 Mobile Safari/537.36", "Android 14", "Chrome 120.0.6099.43"},
		{"curl", "curl/8.4.0", "Unknown", "curl 8.4.0"},
		{"empty", "", "Unknown", "Unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectOS(c.ua); got != c.os {
				t.Errorf("detectOS = %q, want %q", got, c.os)
			}
			if got := detectBrowser(c.ua); got != c.browser {
				t.Errorf("detectBrowser = %q, want %q", got, c.browser)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:51234"
	if ip, _ := clientIP(r); ip != "10.0.0.5" {
		t.Errorf("remote addr: got %q", ip)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if ip, _ := clientIP(r); ip != "203.0.113.7" {
		t.Errorf("forwarded: got %q", ip)
	}
}

func TestHandlerEscapesHTML(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("User-Agent", "<script>alert(1)</script>")
	w := httptest.NewRecorder()
	handleClientInfo(w, r)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, "Client Information") {
		t.Fatalf("unexpected response %d", w.Code)
	}
	if strings.Contains(body, "<script>alert(1)") {
		t.Error("user agent was not HTML-escaped")
	}
}

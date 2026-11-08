// Command showip serves an HTML page describing the requesting client:
// IP address, user agent, operating system and browser.
package main

import (
	"context"
	"errors"
	"flag"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
)

// ClientInfo holds everything rendered back to the caller.
type ClientInfo struct {
	IP          string
	RemoteAddr  string
	ForwardedIP string
	UserAgent   string
	OS          string
	Browser     string
	Method      string
	Host        string
	Path        string
	Proto       string
	Language    string
	Headers     []Header
	Time        string
}

// Header is a single request header, flattened for ordered rendering.
type Header struct {
	Name  string
	Value string
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Client Info</title>
<style>
  :root { --bg:#f6f7f9; --fg:#1d2330; --muted:#5c6577; --card:#fff; --line:#e2e5ea; --accent:#2f6fdb; }
  @media (prefers-color-scheme: dark) {
    :root { --bg:#14171c; --fg:#e6e9ef; --muted:#9aa3b2; --card:#1c2027; --line:#2c323c; --accent:#6ea0ff; }
  }
  body { margin:0; background:var(--bg); color:var(--fg); font:15px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif; }
  main { max-width:820px; margin:0 auto; padding:32px 16px; }
  h1 { font-size:22px; margin:0 0 4px; }
  p.sub { color:var(--muted); margin:0 0 24px; }
  .card { background:var(--card); border:1px solid var(--line); border-radius:10px; padding:4px 16px; margin-bottom:20px; }
  table { width:100%; border-collapse:collapse; }
  th, td { text-align:left; padding:10px 0; border-bottom:1px solid var(--line); vertical-align:top; }
  tr:last-child th, tr:last-child td { border-bottom:0; }
  th { width:180px; color:var(--muted); font-weight:500; padding-right:16px; }
  td { word-break:break-word; font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:13.5px; }
  .big { font-size:17px; color:var(--accent); font-weight:600; }
  h2 { font-size:15px; color:var(--muted); font-weight:600; margin:28px 0 8px; }
</style>
</head>
<body>
<main>
  <h1>Client Information</h1>
  <p class="sub">Generated {{.Time}}</p>
  <div class="card">
    <table>
      <tr><th>Client IP</th><td class="big">{{.IP}}</td></tr>
      <tr><th>Operating System</th><td>{{.OS}}</td></tr>
      <tr><th>Browser</th><td>{{.Browser}}</td></tr>
      <tr><th>User Agent</th><td>{{.UserAgent}}</td></tr>
      <tr><th>Accept-Language</th><td>{{.Language}}</td></tr>
    </table>
  </div>
  <h2>Connection</h2>
  <div class="card">
    <table>
      <tr><th>Remote Address</th><td>{{.RemoteAddr}}</td></tr>
      <tr><th>Forwarded For</th><td>{{if .ForwardedIP}}{{.ForwardedIP}}{{else}}-{{end}}</td></tr>
      <tr><th>Method</th><td>{{.Method}}</td></tr>
      <tr><th>Host</th><td>{{.Host}}</td></tr>
      <tr><th>Path</th><td>{{.Path}}</td></tr>
      <tr><th>Protocol</th><td>{{.Proto}}</td></tr>
    </table>
  </div>
  <h2>Request Headers</h2>
  <div class="card">
    <table>
      {{range .Headers}}<tr><th>{{.Name}}</th><td>{{.Value}}</td></tr>
      {{end}}
    </table>
  </div>
</main>
</body>
</html>
`))

func main() {
	addr := flag.String("addr", envOr("LISTEN_ADDR", ":8080"), "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", handleClientInfo)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("showip listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	log.Print("server stopped")
}

func handleClientInfo(w http.ResponseWriter, r *http.Request) {
	info := collectClientInfo(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := page.Execute(w, info); err != nil {
		log.Printf("template render failed: %v", err)
	}
}

func collectClientInfo(r *http.Request) ClientInfo {
	ua := r.UserAgent()
	ip, forwarded := clientIP(r)

	headers := make([]Header, 0, len(r.Header))
	for name, values := range r.Header {
		headers = append(headers, Header{Name: name, Value: strings.Join(values, ", ")})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })

	return ClientInfo{
		IP:          ip,
		RemoteAddr:  r.RemoteAddr,
		ForwardedIP: forwarded,
		UserAgent:   orDash(ua),
		OS:          detectOS(ua),
		Browser:     detectBrowser(ua),
		Method:      r.Method,
		Host:        r.Host,
		Path:        r.URL.RequestURI(),
		Proto:       r.Proto,
		Language:    orDash(r.Header.Get("Accept-Language")),
		Headers:     headers,
		Time:        time.Now().UTC().Format(time.RFC1123),
	}
}

// clientIP returns the best-guess originating client IP. Behind a Kubernetes
// Service/Ingress the TCP peer is the proxy, so X-Forwarded-For (first hop)
// and X-Real-IP are preferred when present. These headers are client-supplied
// and can be spoofed if the app is reached without a trusted proxy in front.
func clientIP(r *http.Request) (ip, forwarded string) {
	forwarded = r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if net.ParseIP(first) != nil {
			return first, forwarded
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(real) != nil {
		return real, forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr, forwarded
	}
	return host, forwarded
}

// detectOS identifies the operating system from a User-Agent string.
// Order matters: iOS UAs contain "Mac OS X" and Android UAs contain "Linux".
func detectOS(ua string) string {
	switch {
	case ua == "":
		return "Unknown"
	case strings.Contains(ua, "Windows NT 10.0"):
		return "Windows 10/11"
	case strings.Contains(ua, "Windows NT 6.3"):
		return "Windows 8.1"
	case strings.Contains(ua, "Windows NT 6.2"):
		return "Windows 8"
	case strings.Contains(ua, "Windows NT 6.1"):
		return "Windows 7"
	case strings.Contains(ua, "Windows"):
		return "Windows"
	case strings.Contains(ua, "iPhone"):
		return withVersion("iOS", ua, "OS ", " like", "_")
	case strings.Contains(ua, "iPad"):
		return withVersion("iPadOS", ua, "OS ", " like", "_")
	case strings.Contains(ua, "Android"):
		return withVersion("Android", ua, "Android ", ";", "")
	case strings.Contains(ua, "CrOS"):
		return "ChromeOS"
	case strings.Contains(ua, "Mac OS X"):
		return withVersion("macOS", ua, "Mac OS X ", ")", "_")
	case strings.Contains(ua, "Ubuntu"):
		return "Ubuntu Linux"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	case strings.Contains(ua, "FreeBSD"):
		return "FreeBSD"
	default:
		return "Unknown"
	}
}

// detectBrowser identifies the browser from a User-Agent string.
// Order matters: Edge/Opera/Samsung include "Chrome", and Chrome includes "Safari".
func detectBrowser(ua string) string {
	type rule struct{ token, name string }
	rules := []rule{
		{"Edg/", "Microsoft Edge"},
		{"EdgiOS/", "Microsoft Edge"},
		{"EdgA/", "Microsoft Edge"},
		{"OPR/", "Opera"},
		{"SamsungBrowser/", "Samsung Internet"},
		{"Vivaldi/", "Vivaldi"},
		{"YaBrowser/", "Yandex Browser"},
		{"FxiOS/", "Firefox"},
		{"Firefox/", "Firefox"},
		{"CriOS/", "Chrome"},
		{"Chrome/", "Chrome"},
		{"Version/", "Safari"}, // real Safari sends Version/x.y ... Safari/
		{"MSIE ", "Internet Explorer"},
		{"Trident/", "Internet Explorer"},
		{"curl/", "curl"},
		{"Wget/", "Wget"},
		{"PostmanRuntime/", "Postman"},
		{"kube-probe/", "Kubernetes Probe"},
	}
	if ua == "" {
		return "Unknown"
	}
	for _, rl := range rules {
		if rl.token == "Version/" && !strings.Contains(ua, "Safari/") {
			continue
		}
		if i := strings.Index(ua, rl.token); i >= 0 {
			if v := versionAfter(ua[i+len(rl.token):]); v != "" && rl.token != "Trident/" {
				return rl.name + " " + v
			}
			return rl.name
		}
	}
	lower := strings.ToLower(ua)
	if strings.Contains(lower, "bot") || strings.Contains(lower, "spider") || strings.Contains(lower, "crawl") {
		return "Bot/Crawler"
	}
	return "Unknown"
}

// versionAfter returns the leading version token (e.g. "120.0.6099.71").
func versionAfter(s string) string {
	end := strings.IndexFunc(s, func(c rune) bool {
		return !(c >= '0' && c <= '9' || c == '.')
	})
	if end == -1 {
		end = len(s)
	}
	return strings.Trim(s[:end], ".")
}

// withVersion appends a version found between start and stop markers.
func withVersion(name, ua, start, stop, sep string) string {
	i := strings.Index(ua, start)
	if i < 0 {
		return name
	}
	rest := ua[i+len(start):]
	if j := strings.Index(rest, stop); j >= 0 {
		rest = rest[:j]
	}
	if sep != "" {
		rest = strings.ReplaceAll(rest, sep, ".")
	}
	rest = strings.TrimSpace(rest)
	if rest == "" || versionAfter(rest) == "" {
		return name
	}
	return name + " " + versionAfter(rest)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/healthz" {
			log.Printf("%s %s %s %q %s", r.RemoteAddr, r.Method, r.URL.Path, r.UserAgent(), time.Since(start))
		}
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

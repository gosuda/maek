package agent

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestScrapeTargetMetadata(t *testing.T) {
	// 1. Mock server with OG tags
	ogMux := http.NewServeMux()
	ogMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
	<meta property="og:site_name" content="My Great App">
	<title>Fallback Title</title>
	<meta property="og:description" content="Awesome OG Description">
	<meta property="og:image" content="/images/og.png">
</head>
<body><h1>Hello</h1></body>
</html>
`))
	})
	ogMux.HandleFunc("/images/og.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("fake png content"))
	})
	ogServer := httptest.NewServer(ogMux)
	defer ogServer.Close()

	ogURL, _ := url.Parse(ogServer.URL)
	name, desc, thumb := ScrapeTargetMetadata(ogURL)

	if name != "My Great App" {
		t.Errorf("expected 'My Great App', got %q", name)
	}
	if desc != "Awesome OG Description" {
		t.Errorf("expected 'Awesome OG Description', got %q", desc)
	}
	if !strings.HasPrefix(thumb, "data:image/png;base64,") {
		t.Errorf("expected thumb to be data URI, got %q", thumb)
	}

	// 2. Mock server without OG, with title and favicon
	fallbackMux := http.NewServeMux()
	fallbackMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
	<title>Clean Fallback Title</title>
	<link rel="icon" href="/favicon.svg">
</head>
<body><h1>Hello</h1></body>
</html>
`))
	})
	fallbackMux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte("<svg></svg>"))
	})
	fallbackServer := httptest.NewServer(fallbackMux)
	defer fallbackServer.Close()

	fbURL, _ := url.Parse(fallbackServer.URL)
	name2, desc2, thumb2 := ScrapeTargetMetadata(fbURL)

	if name2 != "Clean Fallback Title" {
		t.Errorf("expected 'Clean Fallback Title', got %q", name2)
	}
	if !strings.HasPrefix(thumb2, "data:image/svg+xml;base64,") {
		t.Errorf("expected thumb to be svg data URI, got %q", thumb2)
	}
	_ = desc2
}

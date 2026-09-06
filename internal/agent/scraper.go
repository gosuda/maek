package agent

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type ScrapedMeta struct {
	Title       string
	OGDesc      string
	MetaDesc    string
	OGImage     string
	AppleIcon   string
	Favicon     string
	FinalURL    *url.URL
}

// ScrapeTargetMetadata attempts to extract Open Graph metadata, title, and favicon from the target.
func ScrapeTargetMetadata(targetURL *url.URL) (desc string, thumb string) {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", targetURL.String(), nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "maek-agent/metadata-scraper")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL
	if finalURL == nil {
		finalURL = targetURL
	}

	cType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(cType, "text/html") && !strings.Contains(cType, "application/xhtml+xml") {
		return "", ""
	}

	// Read up to 256KB of HTML
	lr := io.LimitReader(resp.Body, 256*1024)
	meta := parseHTMLMeta(lr, finalURL)

	// 1. Resolve Description: OG Description > Meta Description > Title
	if meta.OGDesc != "" {
		desc = meta.OGDesc
	} else if meta.MetaDesc != "" {
		desc = meta.MetaDesc
	} else if meta.Title != "" {
		desc = meta.Title
	}

	if len(desc) > 160 {
		desc = strings.TrimSpace(desc[:157]) + "..."
	}

	// 2. Resolve Thumbnail / Icon: OG Image > Apple Touch Icon > Favicon
	rawIconURL := ""
	if meta.OGImage != "" {
		rawIconURL = meta.OGImage
	} else if meta.AppleIcon != "" {
		rawIconURL = meta.AppleIcon
	} else if meta.Favicon != "" {
		rawIconURL = meta.Favicon
	}

	if rawIconURL != "" {
		thumb = fetchIconAsDataURI(client, finalURL, rawIconURL)
	}

	return desc, thumb
}

// parseHTMLMeta tokenizes HTML and extracts Open Graph, title, and link tags.
func parseHTMLMeta(r io.Reader, baseURL *url.URL) ScrapedMeta {
	var meta ScrapedMeta
	meta.FinalURL = baseURL

	tokenizer := html.NewTokenizer(r)
	inTitle := false

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return meta

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()

			switch token.Data {
			case "title":
				inTitle = true

			case "meta":
				var prop, name, content string
				for _, attr := range token.Attr {
					key := strings.ToLower(attr.Key)
					val := strings.TrimSpace(attr.Val)
					if key == "property" {
						prop = strings.ToLower(val)
					} else if key == "name" {
						name = strings.ToLower(val)
					} else if key == "content" {
						content = val
					}
				}

				if content != "" {
					if (prop == "og:description" || name == "og:description") && meta.OGDesc == "" {
						meta.OGDesc = content
					} else if name == "description" && meta.MetaDesc == "" {
						meta.MetaDesc = content
					} else if (prop == "og:image" || name == "og:image") && meta.OGImage == "" {
						meta.OGImage = content
					}
				}

			case "link":
				var rel, href string
				for _, attr := range token.Attr {
					key := strings.ToLower(attr.Key)
					val := strings.TrimSpace(attr.Val)
					if key == "rel" {
						rel = strings.ToLower(val)
					} else if key == "href" {
						href = val
					}
				}

				if href != "" {
					if strings.Contains(rel, "apple-touch-icon") && meta.AppleIcon == "" {
						meta.AppleIcon = href
					} else if strings.Contains(rel, "icon") && meta.Favicon == "" {
						meta.Favicon = href
					}
				}
			}

		case html.TextToken:
			if inTitle && meta.Title == "" {
				meta.Title = strings.TrimSpace(tokenizer.Token().Data)
			}

		case html.EndTagToken:
			if tokenizer.Token().Data == "title" {
				inTitle = false
			} else if tokenizer.Token().Data == "head" {
				// We can stop parsing once head ends
				return meta
			}
		}
	}
}

// fetchIconAsDataURI resolves the icon URL and, if small (<= 24KB), encodes it into a Data URI.
func fetchIconAsDataURI(client *http.Client, baseURL *url.URL, rawURL string) string {
	if strings.HasPrefix(rawURL, "data:") {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	resolved := baseURL.ResolveReference(parsed)

	// Fetch icon
	req, err := http.NewRequest("GET", resolved.String(), nil)
	if err != nil {
		return resolved.String()
	}
	req.Header.Set("User-Agent", "maek-agent/metadata-scraper")

	resp, err := client.Do(req)
	if err != nil {
		return resolved.String()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return resolved.String()
	}

	// Limit to 24KB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 24*1024))
	if err != nil || len(body) == 0 {
		return resolved.String()
	}

	cType := resp.Header.Get("Content-Type")
	if cType == "" {
		if strings.HasSuffix(strings.ToLower(resolved.Path), ".svg") {
			cType = "image/svg+xml"
		} else if strings.HasSuffix(strings.ToLower(resolved.Path), ".ico") {
			cType = "image/x-icon"
		} else {
			cType = http.DetectContentType(body)
		}
	} else {
		// Clean up charset if present e.g. "image/svg+xml; charset=utf-8"
		if idx := strings.Index(cType, ";"); idx != -1 {
			cType = strings.TrimSpace(cType[:idx])
		}
	}

	// Format as Data URI
	b64 := base64.StdEncoding.EncodeToString(body)
	return fmt.Sprintf("data:%s;base64,%s", cType, b64)
}

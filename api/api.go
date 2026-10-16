// Package api holds the OpenAPI contract and serves it with Swagger UI.
//
// The specification is written by hand (contract-first) and embedded in the
// binary. A test in internal/app fails when a route is added to the router
// without being documented here, or the other way around.
package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var spec []byte

// Spec returns the raw OpenAPI document.
func Spec() []byte { return spec }

// SpecHandler serves openapi.yaml.
func SpecHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(spec)
}

// Swagger UI assets are pinned and checked with Subresource Integrity.
const docsPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>API documentation</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.33.3/swagger-ui.css"
        integrity="sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW" crossorigin="anonymous">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.33.3/swagger-ui-bundle.js"
          integrity="sha384-h00llMzD1afyNow1pDmnZKlHsUZgp9KE9HXLFXF2CvbwsBY0+WBO4f75SfGwvM4K" crossorigin="anonymous"></script>
  <script src="/docs/init.js"></script>
</body>
</html>
`

const docsInit = `window.ui = SwaggerUIBundle({ url: "/openapi.yaml", dom_id: "#swagger-ui", persistAuthorization: false });`

// DocsHandler serves the Swagger UI page and its init script.
func DocsHandler(w http.ResponseWriter, r *http.Request) {
	// Relax the API-wide CSP for this page only.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self' https://cdn.jsdelivr.net; style-src https://cdn.jsdelivr.net 'unsafe-inline'; "+
			"img-src 'self' data: https://cdn.jsdelivr.net; connect-src 'self'; frame-ancestors 'none'")
	if r.URL.Path == "/docs/init.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(docsInit))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsPage))
}

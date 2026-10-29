package mcp

import (
	"context"
	"io/fs"

	"github.com/greg0x46/julius/internal/webui"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const portfolioAppURI = "ui://julius/investment-portfolio.html"
const appMIME = "text/html;profile=mcp-app"

func registerPortfolioApp(server *sdk.Server) string {
	assets, err := webui.DistFS()
	if err != nil {
		return ""
	}
	html, err := fs.ReadFile(assets, "mcp-investments.html")
	if err != nil {
		return ""
	}
	return addPortfolioApp(server, string(html))
}

func addPortfolioApp(server *sdk.Server, html string) string {
	server.AddResource(&sdk.Resource{URI: portfolioAppURI, Name: "investment_portfolio", MIMEType: appMIME, Description: "Optional interactive read-only portfolio snapshot."}, func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: portfolioAppURI, MIMEType: appMIME, Text: html, Meta: sdk.Meta{"ui": map[string]any{"prefersBorder": true, "csp": map[string]any{"connectDomains": []string{}, "resourceDomains": []string{}, "frameDomains": []string{}, "baseUriDomains": []string{}}}}}}}, nil
	})
	return portfolioAppURI
}

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"airoxy-linux/internal/config"
	"airoxy-linux/internal/proxy"
	"airoxy-linux/internal/rewriter"
	"airoxy-linux/internal/server"

	"github.com/elazarl/goproxy"
)

func main() {
	if err := config.Init(); err != nil {
		log.Fatalf("Failed to init config: %v", err)
	}

	if err := proxy.Init(); err != nil {
		log.Fatalf("Failed to init proxy: %v", err)
	}

	initialCfg := config.Get()
	proxy.ProxyInstance.Verbose = initialCfg.Debug

	go func() {
		for cfg := range config.ConfigUpdateChan {
			proxy.ProxyInstance.Verbose = cfg.Debug
		}
	}()

	proxy.ProxyInstance.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if resp != nil {
			startTime, _ := ctx.UserData.(time.Time)
			origHost := ctx.Req.Header.Get("X-Original-Host")
			displayHost := origHost
			if displayHost == "" {
				displayHost = strings.Split(ctx.Req.URL.Host, ":")[0]
			}

			rule := "Passthrough"
			upstream := "Direct"
			if origHost != "" {
				rule = "Matched"
				upstream = strings.Split(ctx.Req.URL.Host, ":")[0]
			}

			server.PushEvent(displayHost, rule, upstream, time.Since(startTime), resp.StatusCode)
		}
		return resp
	})

	proxy.ProxyInstance.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		ctx.UserData = time.Now()
		host := strings.Split(req.URL.Host, ":")[0]
		cfg := config.Get()

		isInMITMList := false
		for _, pattern := range cfg.MITMHosts {
			if matchHost(pattern, host) {
				isInMITMList = true
				break
			}
		}

		if !cfg.RedirectAll && !isInMITMList {
			return req, nil
		}

		var matchedGroup *config.UpstreamGroup
		var remainderGroup *config.UpstreamGroup

		for i := range cfg.Upstreams {
			group := &cfg.Upstreams[i]
			if group.IsRemainder {
				remainderGroup = group
				continue
			}
			for _, pattern := range group.Hosts {
				if matchHost(pattern, host) {
					matchedGroup = group
					break
				}
			}
			if matchedGroup != nil {
				break
			}
		}

		if matchedGroup == nil && remainderGroup != nil {
			matchedGroup = remainderGroup
		}

		if matchedGroup != nil {
			targetURL := matchedGroup.BaseURL
			scheme := "https"

			if strings.HasPrefix(targetURL, "http://") {
				scheme = "http"
				targetURL = strings.TrimPrefix(targetURL, "http://")
			} else if strings.HasPrefix(targetURL, "https://") {
				scheme = "https"
				targetURL = strings.TrimPrefix(targetURL, "https://")
			}
			targetURL = strings.TrimSuffix(targetURL, "/")

			req.Header.Set("X-Original-Host", host)
			req.Host = targetURL
			req.URL.Host = targetURL
			req.URL.Scheme = scheme

			for _, headerRule := range matchedGroup.Headers {
				if headerRule.Action == "SET" {
					req.Header.Set(headerRule.Name, headerRule.Value)
				} else if headerRule.Action == "REMOVE" {
					req.Header.Del(headerRule.Name)
				}
			}

			// Optimization: Read JSON Body only when rewrites exist
			if req.Method == "POST" && req.Header.Get("Content-Type") == "application/json" && len(matchedGroup.Rewrites) > 0 {
				limitReader := io.LimitReader(req.Body, 2*1024*1024)
				body, err := io.ReadAll(limitReader)
				if err == nil && len(body) > 0 {
					newBody, err := rewriter.RewriteJSON(body, matchedGroup.Rewrites)
					if err == nil {
						req.Body = io.NopCloser(bytes.NewReader(newBody))
						req.ContentLength = int64(len(newBody))
					} else {
						// Restore original body on rewrite failure
						req.Body = io.NopCloser(bytes.NewReader(body))
					}
				}
			}
		}

		return req, nil
	})

	go func() {
		cfg := config.Get()
		fmt.Printf("Admin UI starting on :%d/ui/\n", cfg.AdminPort)
		server.StartAdmin(cfg.AdminPort)
	}()

	initialCfgFinal := config.Get()
	fmt.Printf("Proxy Engine starting on :%d\n", initialCfgFinal.ProxyPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", initialCfgFinal.ProxyPort), proxy.ProxyInstance))
}

func matchHost(pattern, host string) bool {
	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		return strings.HasSuffix(host, suffix)
	}
	return false
}
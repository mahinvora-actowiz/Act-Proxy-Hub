package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"

	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func fetchHandlerActOne(ctx *fasthttp.RequestCtx, apiKey string, tokenID primitive.ObjectID, totalEstimatedCredits int, start time.Time) {
	body := ctx.PostBody()
	if len(body) == 0 {
		sendJSONResponse(ctx, 400, false, "Request body is empty", nil, "proxy_hub")
		return
	}

	type FetchRequest struct {
		URL      string                 `json:"url"`
		Params   map[string]interface{} `json:"params"`
		Method   string                 `json:"method"`
		Payload  interface{}            `json:"payload"`
		Headers  map[string]interface{} `json:"headers"`
		Contains string                 `json:"contains"`
	}

	var req FetchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("❌ ActOne JSON Unmarshal Error: %v", err)
		sendJSONResponse(ctx, 400, false, "Invalid JSON format", nil, "proxy_hub")
		return
	}

	if req.URL == "" {
		sendJSONResponse(ctx, 400, false, "Missing required field: url", nil, "proxy_hub")
		return
	}

	urlStr := req.URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		logRequest(ctx, apiKey, tokenID, urlStr, 400, start, 0, fmt.Sprintf("Invalid URL format: %v", err), "proxy_hub")
		sendJSONResponse(ctx, 400, false, "Invalid URL format", nil, "proxy_hub")
		return
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		logRequest(ctx, apiKey, tokenID, urlStr, 400, start, 0, "Invalid target URL: missing scheme or host", "proxy_hub")
		sendJSONResponse(ctx, 400, false, "Invalid target URL: missing scheme or host", nil, "proxy_hub")
		return
	}

	domain := parsedURL.Hostname()
	if domain != "" {
		go func() {
			if err := addDomainToAlias(collection, apiKey, domain); err != nil {
				log.Printf("⚠️ Failed to add domain %s for key %s: %v", domain, apiKey, err)
			}
		}()
	}

	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "GET"
	}

	actOneReq := map[string]interface{}{
		"url":            req.URL,
		"method":         method,
		"custom_headers": len(req.Headers) > 0,
	}

	if req.Params != nil {
		actOneReq["params"] = req.Params
	}
	if req.Headers != nil {
		actOneReq["headers"] = req.Headers
	}
	if req.Contains != "" {
		actOneReq["contains"] = req.Contains
	}

	if req.Payload != nil {
		if _, ok := req.Payload.(string); ok {
			actOneReq["data"] = req.Payload
		} else {
			actOneReq["json"] = req.Payload
		}
	}

	outgoingHeaders := map[string]string{
		"X-API-Key": apiKey,
	}
	customHeadersStr, _ := json.Marshal(outgoingHeaders)

	executeRequest(
		ctx,
		apiKey,
		tokenID,
		"",
		ActOneEndpoint,
		req.URL,
		"POST",
		actOneReq,
		string(customHeadersStr),
		totalEstimatedCredits,
		start,
		"actone",
	)
}

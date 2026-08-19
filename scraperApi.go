package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func fetchHandlerScraperAPI(ctx *fasthttp.RequestCtx, scraperapiKey string, tokenID primitive.ObjectID, proxyToken string, totalEstimatedCredits int, cfg *ScraperAPIConfig, start time.Time) {
	body := ctx.PostBody()
	if len(body) == 0 {
		sendJSONResponse(ctx, 400, false, "Request body is empty", nil)
		return
	}

	// 1. Define the expected JSON structure (Matches ScrapeDo exactly)
	type FetchRequest struct {
		URL         string                 `json:"url"`
		Params      map[string]interface{} `json:"params"`
		Method      string                 `json:"method"`
		Mode        string                 `json:"mode"`
		Payload     interface{}            `json:"payload"`
		SetCookies  interface{}            `json:"setCookies"`
		Headers     map[string]interface{} `json:"headers"`
		ProxyParams map[string]interface{} `json:"proxy_params"`
	}

	var req FetchRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber() // Tells Go to keep numbers as raw text, preventing precision loss
	if err := decoder.Decode(&req); err != nil {
		log.Printf("❌ JSON Unmarshal Error: %v | Body: %s", err, string(body))
		sendJSONResponse(ctx, 400, false, fmt.Sprintf("Invalid JSON format: %v", err), nil)
		return
	}

	// Normalize method to uppercase (e.g., "post" -> "POST") to prevent proxy errors
	method := strings.ToUpper(req.Method)
	mode := strings.ToLower(req.Mode)

	if method == "" {
		method = "GET"
	}
	if mode == "" {
		mode = "proxy"
	}

	// 2. Process Payload
	var payloadStr string
	if req.Payload != nil {
		if payloadMap, ok := req.Payload.(map[string]interface{}); ok {
			// If it's a dictionary, convert it to a raw JSON string.
			jsonBytes, _ := json.Marshal(payloadMap)
			payloadStr = string(jsonBytes)
		} else if payloadStrVal, ok := req.Payload.(string); ok {
			// If the user already sent a raw string, just use it as-is.
			payloadStr = payloadStrVal
		}
	}

	// 3. Process Headers
	var customHeaders string
	if req.Headers != nil {
		if headerBytes, err := json.Marshal(req.Headers); err == nil {
			customHeaders = string(headerBytes)
		}
	}

	// 4. Initialize Config safely
	if cfg == nil {
		cfg = &ScraperAPIConfig{}
	}
	if cfg.Token == "" {
		cfg.Token = proxyToken
	}

	// 5. Apply ProxyParams cleanly using helper functions
	if req.ProxyParams != nil {
		// Booleans
		if v := getBoolPtr(req.ProxyParams, "render"); v != nil {
			cfg.Render = v
		}
		if v := getBoolPtr(req.ProxyParams, "screenshot"); v != nil {
			cfg.Screenshot = v
		}
		if v := getBoolPtr(req.ProxyParams, "premium"); v != nil {
			cfg.Premium = v
		}
		if v := getBoolPtr(req.ProxyParams, "ultra_premium"); v != nil {
			cfg.UltraPremium = v
		}
		if v := getBoolPtr(req.ProxyParams, "keep_headers"); v != nil {
			cfg.KeepHeaders = v
		}
		if v := getBoolPtr(req.ProxyParams, "autoparse"); v != nil {
			cfg.AutoParse = v
		}
		if v := getBoolPtr(req.ProxyParams, "follow_redirect"); v != nil {
			cfg.FollowRedirect = v
		}

		// Strings
		if v := getStringVal(req.ProxyParams, "wait_for_selector"); v != "" {
			cfg.WaitForSelector = v
		}
		if v := getStringVal(req.ProxyParams, "country_code"); v != "" {
			cfg.CountryCode = v
		}
		if v := getStringVal(req.ProxyParams, "device_type"); v != "" {
			cfg.DeviceType = v
		}
		if v := getStringVal(req.ProxyParams, "output_format"); v != "" {
			cfg.OutputFormat = v
		}

		// Integers
		if v := getIntPtr(req.ProxyParams, "session_number"); v != nil {
			cfg.SessionNumber = v
		}
		if v := getIntPtr(req.ProxyParams, "timeout"); v != nil {
			cfg.Timeout = *v
		}
	}

	// 6. Parse and validate target URL
	urlStr := req.URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		sendJSONResponse(ctx, 400, false, "Invalid URL format", nil)
		return
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		sendJSONResponse(ctx, 400, false, "Invalid target URL: missing scheme or host", nil)
		return
	}

	domain := parsedURL.Hostname()
	if domain != "" {
		go func() {
			if err := addDomainToAlias(collection, scraperapiKey, domain); err != nil {
				log.Printf("⚠️ Failed to add domain %s for key %s: %v", domain, scraperapiKey, err)
			}
		}()
	}

	// 7. Merge req.Params into the target URL query string safely
	query := parsedURL.Query()
	if req.Params != nil {
		for k, v := range req.Params {
			switch val := v.(type) {
			case string:
				query.Set(k, val)
			case float64, bool, int:
				query.Set(k, fmt.Sprintf("%v", val))
			default:
				// For complex types (maps, slices), marshal to JSON string
				if b, err := json.Marshal(val); err == nil {
					query.Set(k, string(b))
				} else {
					query.Set(k, fmt.Sprintf("%v", val))
				}
			}
		}
	}
	parsedURL.RawQuery = query.Encode()
	targetURL := parsedURL.String()

	// 8. Execute Request based on mode
	if mode == "api" {
		apiParams := url.Values{}
		apiParams.Set("api_key", cfg.Token)
		apiParams.Set("url", targetURL)

		setBool(apiParams, "render", cfg.Render)
		setBool(apiParams, "screenshot", cfg.Screenshot)
		setString(apiParams, "wait_for_selector", cfg.WaitForSelector)
		setString(apiParams, "country_code", cfg.CountryCode)
		setBool(apiParams, "premium", cfg.Premium)
		setBool(apiParams, "ultra_premium", cfg.UltraPremium)
		setIntPtr(apiParams, "session_number", cfg.SessionNumber)
		setBool(apiParams, "keep_headers", cfg.KeepHeaders)
		setString(apiParams, "device_type", cfg.DeviceType)
		setBool(apiParams, "autoparse", cfg.AutoParse)
		setString(apiParams, "output_format", cfg.OutputFormat)
		setBool(apiParams, "follow_redirect", cfg.FollowRedirect)
		setInt(apiParams, "timeout", cfg.Timeout)

		apiEndpoint := fmt.Sprintf("http://api.scraperapi.com/?%s", apiParams.Encode())
		executeRequest(ctx, scraperapiKey, tokenID, "", apiEndpoint, method, payloadStr, customHeaders, totalEstimatedCredits, start, "scraperapi")
		return
	}

	// Proxy Mode
	optionsStr := buildScraperAPIProxyOptions(cfg)
	proxyAddr := fmt.Sprintf("http://%s:%s@proxy-server.scraperapi.com:8001", optionsStr, cfg.Token)

	executeRequest(ctx, scraperapiKey, tokenID, proxyAddr, targetURL, method, payloadStr, customHeaders, totalEstimatedCredits, start, "scraperapi")
}

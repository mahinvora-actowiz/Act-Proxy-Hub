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

func fetchHandlerScrapeDo(ctx *fasthttp.RequestCtx, scrapedoKey string, tokenID primitive.ObjectID, proxyToken string, totalEstimatedCredits int, cfg *ScrapeDoConfig, start time.Time) {
	body := ctx.PostBody()
	if len(body) == 0 {
		sendJSONResponse(ctx, 400, false, "Request body is empty", nil)
		return
	}

	// 1. Define the expected JSON structure
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

	var payloadStr string
	if req.Payload != nil {
		if payloadMap, ok := req.Payload.(map[string]interface{}); ok {
			jsonBytes, _ := json.Marshal(payloadMap)
			payloadStr = string(jsonBytes)
		} else if payloadSlice, ok := req.Payload.([]interface{}); ok {
			// Handle top-level JSON arrays
			jsonBytes, _ := json.Marshal(payloadSlice)
			payloadStr = string(jsonBytes)
		} else if payloadStrVal, ok := req.Payload.(string); ok {
			payloadStr = payloadStrVal
		}
	}

	// 3. Process SetCookies
	var rawCookies string
	if req.SetCookies != nil {
		if cookiesStr, ok := req.SetCookies.(string); ok {
			rawCookies = cookiesStr
		} else {
			if jsonBytes, err := json.Marshal(req.SetCookies); err == nil {
				rawCookies = string(jsonBytes)
			}
		}
	}

	// 4. Process Headers
	var customHeaders string
	if req.Headers != nil {
		if headerBytes, err := json.Marshal(req.Headers); err == nil {
			customHeaders = string(headerBytes)
		}
	}

	// 5. Initialize Config safely
	if cfg == nil {
		cfg = &ScrapeDoConfig{}
	}
	if cfg.Token == "" {
		cfg.Token = proxyToken
	}

	// 6. Apply ProxyParams cleanly using helper functions
	if req.ProxyParams != nil {
		// Booleans
		if v := getBoolPtr(req.ProxyParams, "super"); v != nil {
			cfg.Super = v
		}
		if v := getBoolPtr(req.ProxyParams, "render"); v != nil {
			cfg.Render = v
		}
		if v := getBoolPtr(req.ProxyParams, "customHeaders"); v != nil {
			cfg.CustomHeaders = v
		}
		if v := getBoolPtr(req.ProxyParams, "extraHeaders"); v != nil {
			cfg.ExtraHeaders = v
		}
		if v := getBoolPtr(req.ProxyParams, "forwardHeaders"); v != nil {
			cfg.ForwardHeaders = v
		}
		if v := getBoolPtr(req.ProxyParams, "disableRetry"); v != nil {
			cfg.DisableRetry = v
		}
		if v := getBoolPtr(req.ProxyParams, "disableRedirection"); v != nil {
			cfg.DisableRedirection = v
		}
		if v := getBoolPtr(req.ProxyParams, "screenShot"); v != nil {
			cfg.ScreenShot = v
		}
		if v := getBoolPtr(req.ProxyParams, "fullScreenShot"); v != nil {
			cfg.FullScreenShot = v
		}
		if v := getBoolPtr(req.ProxyParams, "returnJSON"); v != nil {
			cfg.ReturnJSON = v
		}
		if v := getBoolPtr(req.ProxyParams, "blockResources"); v != nil {
			cfg.BlockResources = v
		}
		if v := getBoolPtr(req.ProxyParams, "playWithBrowser"); v != nil {
			cfg.PlayWithBrowser = v
		}
		if v := getBoolPtr(req.ProxyParams, "transparentResponse"); v != nil {
			cfg.TransparentResponse = v
		}
		if v := getBoolPtr(req.ProxyParams, "showFrames"); v != nil {
			cfg.ShowFrames = v
		}
		if v := getBoolPtr(req.ProxyParams, "showWebsocketRequests"); v != nil {
			cfg.ShowWebsocketRequests = v
		}
		if v := getBoolPtr(req.ProxyParams, "pureCookies"); v != nil {
			cfg.PureCookies = v
		}

		// Strings
		if v := getStringVal(req.ProxyParams, "geoCode"); v != "" {
			cfg.GeoCode = v
		}
		if v := getStringVal(req.ProxyParams, "regionalGeoCode"); v != "" {
			cfg.RegionalGeoCode = v
		}
		if v := getStringVal(req.ProxyParams, "waitUntil"); v != "" {
			cfg.WaitUntil = v
		}
		if v := getStringVal(req.ProxyParams, "waitSelector"); v != "" {
			cfg.WaitSelector = v
		}
		if v := getStringVal(req.ProxyParams, "device"); v != "" {
			cfg.Device = v
		}
		if v := getStringVal(req.ProxyParams, "particularScreenShot"); v != "" {
			cfg.ParticularScreenShot = v
		}
		if v := getStringVal(req.ProxyParams, "output"); v != "" {
			cfg.Output = v
		}
		if v := getStringVal(req.ProxyParams, "callback"); v != "" {
			cfg.Callback = v
		}

		// Integers
		if v := getIntPtr(req.ProxyParams, "timeout"); v != nil {
			cfg.Timeout = *v
		}
		if v := getIntPtr(req.ProxyParams, "retryTimeout"); v != nil {
			cfg.RetryTimeout = *v
		}
		if v := getIntPtr(req.ProxyParams, "customWait"); v != nil {
			cfg.CustomWait = *v
		}
		if v := getIntPtr(req.ProxyParams, "width"); v != nil {
			cfg.Width = *v
		}
		if v := getIntPtr(req.ProxyParams, "height"); v != nil {
			cfg.Height = *v
		}
		if v := getIntPtr(req.ProxyParams, "sessionId"); v != nil {
			cfg.SessionId = v
		}
	}

	// 7. Parse and validate target URL
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
			if err := addDomainToAlias(collection, scrapedoKey, domain); err != nil {
				log.Printf("⚠️ Failed to add domain %s for key %s: %v", domain, scrapedoKey, err)
			}
		}()
	}

	// 8. Merge req.Params into the target URL query string safely
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
	urlStr = parsedURL.String()

	urlStr = strings.ReplaceAll(urlStr, "%2A", "*")

	// 9. Execute Request based on mode
	if mode == "api" {
		apiParams := url.Values{}
		apiParams.Set("token", cfg.Token)
		apiParams.Set("url", urlStr)

		setBool(apiParams, "super", cfg.Super)
		setBool(apiParams, "render", cfg.Render)
		setString(apiParams, "geoCode", cfg.GeoCode)
		setString(apiParams, "regionalGeoCode", cfg.RegionalGeoCode)
		setIntPtr(apiParams, "sessionId", cfg.SessionId)
		setBool(apiParams, "customHeaders", cfg.CustomHeaders)
		setBool(apiParams, "extraHeaders", cfg.ExtraHeaders)
		setBool(apiParams, "forwardHeaders", cfg.ForwardHeaders)

		if rawCookies != "" {
			encodedCookies, err := ProcessJSONToEncodedCookies(rawCookies)
			if err != nil {
				log.Printf("❌ Cookie validation failed: %v", err)
				sendJSONResponse(ctx, 400, false, fmt.Sprintf("Invalid setCookies: %v", err), nil)
				return
			}
			apiParams.Set("setCookies", encodedCookies)
		}

		setInt(apiParams, "timeout", cfg.Timeout)
		setInt(apiParams, "retryTimeout", cfg.RetryTimeout)
		setBool(apiParams, "disableRetry", cfg.DisableRetry)
		setBool(apiParams, "disableRedirection", cfg.DisableRedirection)
		setString(apiParams, "waitUntil", cfg.WaitUntil)
		setInt(apiParams, "customWait", cfg.CustomWait)
		setString(apiParams, "waitSelector", cfg.WaitSelector)
		setString(apiParams, "device", cfg.Device)
		setInt(apiParams, "width", cfg.Width)
		setInt(apiParams, "height", cfg.Height)
		setBool(apiParams, "screenShot", cfg.ScreenShot)
		setBool(apiParams, "fullScreenShot", cfg.FullScreenShot)
		setString(apiParams, "particularScreenShot", cfg.ParticularScreenShot)
		setString(apiParams, "output", cfg.Output)
		setBool(apiParams, "returnJSON", cfg.ReturnJSON)
		setBool(apiParams, "blockResources", cfg.BlockResources)
		setBool(apiParams, "playWithBrowser", cfg.PlayWithBrowser)
		setBool(apiParams, "transparentResponse", cfg.TransparentResponse)
		setBool(apiParams, "showFrames", cfg.ShowFrames)
		setBool(apiParams, "showWebsocketRequests", cfg.ShowWebsocketRequests)
		setBool(apiParams, "pureCookies", cfg.PureCookies)
		setString(apiParams, "callback", cfg.Callback)

		apiEndpoint := fmt.Sprintf("https://api.scrape.do/?%s", apiParams.Encode())
		executeRequest(ctx, scrapedoKey, tokenID, "", apiEndpoint, method, payloadStr, customHeaders, totalEstimatedCredits, start, "scrapedo")
		return
	}

	// Proxy Mode
	params := url.Values{}
	setBool(params, "super", cfg.Super)
	setBool(params, "render", cfg.Render)
	setString(params, "geoCode", cfg.GeoCode)
	setString(params, "regionalGeoCode", cfg.RegionalGeoCode)
	setIntPtr(params, "sessionId", cfg.SessionId)
	setBool(params, "customHeaders", cfg.CustomHeaders)
	setBool(params, "extraHeaders", cfg.ExtraHeaders)
	setBool(params, "forwardHeaders", cfg.ForwardHeaders)

	if rawCookies != "" {
		encodedCookies, err := ProcessJSONToEncodedCookies(rawCookies)
		if err != nil {
			log.Printf("❌ Cookie validation failed: %v", err)
			sendJSONResponse(ctx, 400, false, fmt.Sprintf("Invalid setCookies: %v", err), nil)
			return
		}
		params.Set("setCookies", encodedCookies)
	}

	setInt(params, "timeout", cfg.Timeout)
	setInt(params, "retryTimeout", cfg.RetryTimeout)
	setBool(params, "disableRetry", cfg.DisableRetry)
	setBool(params, "disableRedirection", cfg.DisableRedirection)
	setString(params, "waitUntil", cfg.WaitUntil)
	setInt(params, "customWait", cfg.CustomWait)
	setString(params, "waitSelector", cfg.WaitSelector)
	setString(params, "device", cfg.Device)
	setInt(params, "width", cfg.Width)
	setInt(params, "height", cfg.Height)
	setBool(params, "screenShot", cfg.ScreenShot)
	setBool(params, "fullScreenShot", cfg.FullScreenShot)
	setString(params, "particularScreenShot", cfg.ParticularScreenShot)
	setString(params, "output", cfg.Output)
	setBool(params, "returnJSON", cfg.ReturnJSON)
	setBool(params, "blockResources", cfg.BlockResources)
	setBool(params, "playWithBrowser", cfg.PlayWithBrowser)
	setBool(params, "transparentResponse", cfg.TransparentResponse)
	setBool(params, "showFrames", cfg.ShowFrames)
	setBool(params, "showWebsocketRequests", cfg.ShowWebsocketRequests)
	setBool(params, "pureCookies", cfg.PureCookies)
	setString(params, "callback", cfg.Callback)

	proxyPass := params.Encode()
	proxyAddr := fmt.Sprintf("http://%s:%s@proxy.scrape.do:8080", cfg.Token, proxyPass)

	executeRequest(ctx, scrapedoKey, tokenID, proxyAddr, urlStr, method, payloadStr, customHeaders, totalEstimatedCredits, start, "scrapedo")
}

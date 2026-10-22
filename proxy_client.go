package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var proxyClients sync.Map

func getProxyClient(proxyAddr string) *fasthttp.Client {
	if val, ok := proxyClients.Load(proxyAddr); ok {
		return val.(*fasthttp.Client)
	}

	var client = &fasthttp.Client{
		Dial: fasthttpproxy.FasthttpHTTPDialerTimeout(proxyAddr, 10*time.Second),

		MaxConnsPerHost:     500,
		MaxIdleConnDuration: 15 * time.Second,
		MaxConnDuration:     2 * time.Minute,
		MaxConnWaitTimeout:  10 * time.Second,

		ReadBufferSize:  32768,
		WriteBufferSize: 32768,

		ReadTimeout:  125 * time.Second,
		WriteTimeout: 125 * time.Second,

		TLSConfig: &tls.Config{
			InsecureSkipVerify:     true,
			MinVersion:             tls.VersionTLS12,
			SessionTicketsDisabled: true,
		},
	}

	proxyClients.Store(proxyAddr, client)
	return client
}

func doRequestWithRetry(client *fasthttp.Client, req *fasthttp.Request, resp *fasthttp.Response) error {
	var err error
	for i := range 2 {
		start := time.Now()
		log.Printf("🔄 Attempt %d: Sending request to proxy...", i+1)

		err = client.Do(req, resp)
		duration := time.Since(start)

		if err == nil && resp != nil && resp.StatusCode() < 500 {
			log.Printf("✅ Proxy request succeeded in %v", duration)
			return nil
		}

		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
			// log.Printf("❌ Error %v", errMsg)
		}
		log.Printf("❌ Attempt %d FAILED after %v: %v", i+1, duration, errMsg)

		if err != nil {
			if strings.Contains(errMsg, "tls handshake") {
				log.Printf("🔐 TLS HANDSHAKE ERROR - Issue with proxy")
			}
			if strings.Contains(errMsg, "no such host") {
				log.Printf("🌐 DNS ERROR")
			}
			if strings.Contains(errMsg, "closed connection") {
				log.Printf("🔌 CONNECTION CLOSED - Proxy or network issue")
			}
		}

		if err == nil && resp != nil {
			code := resp.StatusCode()
			if code >= 400 && code < 500 {
				return fmt.Errorf("client error: %d", code)
			}
		}

		if i < 1 {
			sleep := time.Duration(100*(1<<i)) * time.Millisecond
			log.Printf("🔄 Retrying in %v...", sleep)
			time.Sleep(sleep)
		}
	}
	return err
}

func executeRequest(ctx *fasthttp.RequestCtx, apiKey string, tokenID primitive.ObjectID, proxyAddr string,
	targetURL string, originalTargetURL string, method string, payload interface{}, customHeaders string,
	totalEstimatedCredits int, start time.Time, provider string) {

	// Variables to capture final state for logging
	var finalStatusCode int
	var finalActualCost int
	var finalErrorMessage string
	var errorSource string = "none"

	// Defer ensures we log the request exactly once, regardless of how the function exits
	defer func() {
		logRequest(ctx, apiKey, tokenID, originalTargetURL, finalStatusCode, start, finalActualCost, finalErrorMessage, errorSource)
	}()

	var client *fasthttp.Client
	if proxyAddr == "" {
		client = &fasthttp.Client{}
	} else {
		client = getProxyClient(proxyAddr)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)
	log.Printf("target=%s", targetURL)

	req.SetRequestURI(targetURL)
	req.URI().DisablePathNormalizing = true

	req.SetTimeout(60 * time.Second)

	if proxyAddr != "" && strings.HasPrefix(strings.ToLower(targetURL), "http://") {
		proxyURL, err := url.Parse(proxyAddr)
		if err == nil && proxyURL.User != nil {
			username := proxyURL.User.Username()
			password, _ := proxyURL.User.Password()

			// Create Basic Auth string
			auth := username + ":" + password
			basicAuth := base64.StdEncoding.EncodeToString([]byte(auth))

			// Set the Proxy-Authorization header
			req.Header.Set("Proxy-Authorization", "Basic "+basicAuth)
		}
	}

	switch strings.ToUpper(method) {
	case "GET":
		req.Header.SetMethod("GET")
	case "POST":
		req.Header.SetMethod("POST")
		var finalPayload []byte
		switch p := payload.(type) {
		case string:
			finalPayload = []byte(p)
			if req.Header.Peek("Content-Type") == nil && len(p) > 0 && (p[0] == '{' || p[0] == '[') {
				req.Header.Set("Content-Type", "application/json")
			}
		case nil:
			finalPayload = []byte("{}")
			req.Header.Set("Content-Type", "application/json")
		default:
			var err error
			finalPayload, err = json.Marshal(p)
			if err != nil {
				log.Printf("❌ Payload Marshal Error: %v", err)
				finalErrorMessage = fmt.Sprintf("Failed to marshal request payload: %v", err)
				finalStatusCode = 500
				errorSource = "proxy_hub" // Gateway failed to process payload
				sendJSONResponse(ctx, 500, false, "Internal server error", nil, "proxy_hub")
				return
			}
			req.Header.Set("Content-Type", "application/json")
		}
		req.SetBody(finalPayload)
	default:
		finalStatusCode = 400
		errorSource = "proxy_hub" // Gateway rejected invalid method
		sendJSONResponse(ctx, 400, false, "Invalid HTTP method", nil, "proxy_hub")
		return
	}

	if customHeaders != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(customHeaders), &headers); err == nil {
			for k, v := range headers {
				lk := strings.ToLower(k)
				if lk == "host" || lk == "content-length" || lk == "connection" ||
					lk == "transfer-encoding" || lk == "upgrade" || lk == "proxy-authorization" {
					continue
				}
				req.Header.Set(k, v)
			}
		}
	}

	req.Header.Set("Connection", "close")

	if proxyAddr != "" {
		log.Printf("🔗 [%s] Connecting | proxy=%s | target=%s | key=%s",
			strings.ToUpper(provider), proxyAddr, targetURL, apiKey)
	} else {
		log.Printf("🔗 [%s] Connecting | target=%s | key=%s",
			strings.ToUpper(provider), targetURL, apiKey)
	}

	err := doRequestWithRetry(client, req, resp)

	if err != nil {
		finalErrorMessage = err.Error()
	}

	// 1. Calculate base cost for providers that report it in headers
	actualCost := 0
	switch provider {
	case "scrapedo":
		if resp != nil {
			if costHeader := string(resp.Header.Peek("scrape.do-request-cost")); costHeader != "" {
				if c, err := strconv.Atoi(costHeader); err == nil {
					actualCost = c
				}
			}
		}
	case "scraperapi":
		if resp != nil {
			if costHeader := string(resp.Header.Peek("sa-credit-cost")); costHeader != "" {
				if c, err := strconv.Atoi(costHeader); err == nil {
					actualCost = c
				}
			}
		}
	}

	// 2. Determine success safely (checks resp != nil first to prevent panic)
	success := err == nil && resp != nil && resp.StatusCode() == 200

	// 3. AtOne: Charge 1 credit ONLY on success, 0 on failure
	if provider == "actone" && success {
		actualCost = 1
	}

	finalActualCost = actualCost

	// 4. Safely extract status code to prevent panic if resp is nil (e.g., network drop)
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode()
	}

	isNetworkError := err != nil
	isProxyFailure := resp != nil && (statusCode == 502 || statusCode == 503 || statusCode == 504)

	if isNetworkError || isProxyFailure {
		errorSource = "proxy_gateway"

		log.Printf("❌ Request failed | provider=%s | url=%s | err=%v | status=%d",
			provider, targetURL, err, statusCode)

		updateInc := bson.M{"usedCredits": actualCost, "total": 1, "fail": 1}
		updateQuery := bson.M{"$inc": updateInc}

		if actualCost > 0 {
			updateQuery["$addToSet"] = bson.M{"distinctCredits": actualCost}
		}

		collection.UpdateOne(context.Background(),
			bson.M{"apiKey": apiKey},
			updateQuery)
		
		if isNetworkError {
			// Capture exact upstream body if available, even on network errors
			if resp != nil && len(resp.Body()) > 0 {
				finalErrorMessage = fmt.Sprintf("Network error: %v | Upstream: %s", err, strings.TrimSpace(string(resp.Body())))
			} else {
				finalErrorMessage = err.Error()
			}
			log.Printf("🚨 NETWORK ERROR (fasthttp failed to connect/dropped): %v | URL: %s", err, targetURL)
		} else if statusCode == 502 {
			// Ensure upstream body is captured for 502
			upstreamBody := strings.TrimSpace(string(resp.Body()))
			if upstreamBody != "" {
				finalErrorMessage = fmt.Sprintf("Proxy 502: %s", upstreamBody)
			} else {
				finalErrorMessage = "Proxy 502: Bad Gateway"
			}
			log.Printf("🚨 PROXY RETURNED 502 HTTP STATUS. Body: %s | URL: %s", upstreamBody, targetURL)
		} else {
			// Fallback for 503/504 or other proxy failures NOW includes exact upstream body
			upstreamBody := strings.TrimSpace(string(resp.Body()))
			if upstreamBody != "" {
				finalErrorMessage = fmt.Sprintf("Proxy failure: HTTP %d | Upstream: %s", statusCode, upstreamBody)
			} else if finalErrorMessage == "" {
				finalErrorMessage = fmt.Sprintf("Proxy failure: HTTP %d", statusCode)
			}
		}

		if err != nil {
			if errors.Is(err, fasthttp.ErrTimeout) {
				finalStatusCode = 504
				sendJSONResponse(ctx, 504, false, "Upstream timeout", nil, "proxy_gateway")
			} else {
				finalStatusCode = 502
				sendJSONResponse(ctx, 502, false, "Bad gateway", nil, "proxy_gateway")
			}
			return
		}

		finalStatusCode = statusCode
		sendJSONResponse(ctx, statusCode, false, "Proxy error", map[string]interface{}{
			"upstreamStatus": statusCode,
			"body":           resp.Body(),
		}, "proxy_gateway")
		return
	}

	// 5. Handle non-proxy failures (e.g., 404, 401, 403, 400)
	updateInc := bson.M{"usedCredits": actualCost, "total": 1}
	if success {
		updateInc["success"] = 1
	} else if statusCode == 429 {
		updateInc["quotaExceeded"] = 1
		upstreamBody := ""
		if resp != nil {
			upstreamBody = strings.TrimSpace(string(resp.Body()))
		}
		if upstreamBody != "" {
			finalErrorMessage = fmt.Sprintf("Upstream quota exceeded (429): %s", upstreamBody)
		} else if finalErrorMessage == "" {
			finalErrorMessage = "Upstream quota exceeded (429)"
		}
	} else {
		updateInc["fail"] = 1
		errorSource = "proxy_gateway"
		// Always capture exact upstream error body for 4xx/5xx to prevent blank messages
		upstreamBody := ""
		if resp != nil {
			upstreamBody = strings.TrimSpace(string(resp.Body()))
		}
		if upstreamBody != "" {
			finalErrorMessage = fmt.Sprintf("Upstream error %d: %s", statusCode, upstreamBody)
		} else if finalErrorMessage == "" {
			finalErrorMessage = fmt.Sprintf("Upstream error %d (empty body)", statusCode)
		}
	}

	filter := bson.M{
		"apiKey": apiKey,
		"total":  bson.M{"$lt": totalEstimatedCredits}, 
	}

	updateQuery := bson.M{
		"$inc": updateInc, 
		"$set": bson.M{"updatedAt": time.Now()},
	}

	if actualCost > 0 {
		updateQuery["$addToSet"] = bson.M{"distinctCredits": actualCost}
	}

	res, err := collection.UpdateOne(
		context.Background(),
		filter,
		updateQuery,
	)

	if err != nil {
		log.Println("️ Mongo stats update failed:", err)
		// Depending on your needs, you might want to return 500 here if the DB is down
	}

	if res != nil && res.ModifiedCount == 0 {
		log.Printf("🚫 Quota exceeded | key=%s | provider=%s | cost=%d", apiKey, provider, actualCost)
		
		// Optional: Record the overage attempt in the DB
		collection.UpdateOne(context.Background(),
			bson.M{"apiKey": apiKey},
			bson.M{"$inc": bson.M{"quotaExceeded": 1}}) // Removed 'total: 1' and 'usedCredits' here to avoid double counting if not desired

		finalStatusCode = 429
		finalErrorMessage = "Quota exceeded"
		errorSource = "proxy_hub"
		
		sendJSONResponse(ctx, 429, false, "Quota exceeded", nil, "proxy_hub")
		return
	}

	if statusCode >= 400 {
		finalStatusCode = statusCode
		
		// Always label upstream errors as proxy_gateway
		errorSource = "proxy_gateway" 
		
		upstreamBody := ""
		if resp != nil {
			upstreamBody = strings.TrimSpace(string(resp.Body()))
		}

		var enrichedData interface{}
		if upstreamBody != "" {
			var upstreamJSON map[string]interface{}
			
			// 1. Try to parse the upstream response as JSON
			if err := json.Unmarshal([]byte(upstreamBody), &upstreamJSON); err == nil {
				// 2. If successful, safely inject our error_source field
				// upstreamJSON["error_source"] = errorSource
				enrichedData = upstreamJSON
			} else {
				// 3. If it's not valid JSON (e.g., raw HTML block page), pass it as a string
				enrichedData = map[string]interface{}{
					"upstream_body": upstreamBody,
				}
			}
		} else {
			enrichedData = map[string]interface{}{
				"upstream_body": "",
			}
		}

		// 🚀 NEW: Log the EXACT upstream response in the errorMessage for MongoDB
		if upstreamBody != "" {
			finalErrorMessage = fmt.Sprintf("Upstream error %d | Exact Response: %s", statusCode, upstreamBody)
		} else {
			finalErrorMessage = fmt.Sprintf("Upstream error %d (empty body)", statusCode)
		}

		sendJSONResponse(ctx, statusCode, false, finalErrorMessage, enrichedData, errorSource)
		return
	}

	log.Printf("✅ Success | provider=%s | status=%d | time=%v | cost=%d",
		provider, statusCode, time.Since(start), actualCost)

	finalStatusCode = statusCode
	errorSource = "proxy_gateway" // Indicate response successfully came through proxy gateway

	upstreamBody := string(resp.Body())

	// Construct the exact response structure you requested
	successResponse := map[string]interface{}{
		"status":       "success",
		"status_code":  statusCode,
		"url":          originalTargetURL,
		"error_source": errorSource,
		"content":      upstreamBody,
	}

	jsonData, err := json.Marshal(successResponse)
	if err != nil {
		log.Printf("❌ Failed to marshal success response: %v", err)
		ctx.SetStatusCode(500)
		ctx.SetBodyString(`{"status":"error","message":"Internal server error"}`)
		return
	}

	// Send the unified success response
		resp.Header.VisitAll(func(k, v []byte) {
		ctx.Response.Header.SetBytesKV(k, v)
	})
	ctx.SetStatusCode(statusCode)
	ctx.SetBody(jsonData)
}
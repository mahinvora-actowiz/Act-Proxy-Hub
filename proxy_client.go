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
		Dial: fasthttpproxy.FasthttpHTTPDialer(proxyAddr),

		MaxConnsPerHost:     300,
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
	targetURL string, method string, payload string, customHeaders string,
	totalEstimatedCredits int, start time.Time, provider string) {

	// Variables to capture final state for logging
	var finalStatusCode int
	var finalActualCost int

	// Defer ensures we log the request exactly once, regardless of how the function exits
	defer func() {
		logRequest(ctx, apiKey, tokenID, targetURL, finalStatusCode, start, finalActualCost)
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
		req.SetBody([]byte(payload))

		if req.Header.Peek("Content-Type") == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	default:
		finalStatusCode = 400
		sendJSONResponse(ctx, 400, false, "Invalid HTTP method", nil)
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

	actualCost := 0
	switch provider {
	case "scrapedo":
		if costHeader := string(resp.Header.Peek("scrape.do-request-cost")); costHeader != "" {
			if c, err := strconv.Atoi(costHeader); err == nil {
				actualCost = c
			}
		}
	case "scraperapi":
		if costHeader := string(resp.Header.Peek("sa-credit-cost")); costHeader != "" {
			if c, err := strconv.Atoi(costHeader); err == nil {
				actualCost = c
			}
		}
	}
	finalActualCost = actualCost

	success := err == nil && resp != nil && resp.StatusCode() == 200
	isNetworkError := err != nil
	isProxyFailure := resp != nil && (resp.StatusCode() == 502 || resp.StatusCode() == 503 || resp.StatusCode() == 504)

	if isNetworkError || isProxyFailure {
		log.Printf("❌ Request failed | provider=%s | url=%s | err=%v | status=%d",
			provider, targetURL, err, resp.StatusCode())

		updateInc := bson.M{"usedCredits": actualCost, "total": 1, "fail": 1}
		collection.UpdateOne(context.Background(),
			bson.M{"apiKey": apiKey},
			bson.M{"$inc": updateInc})

		if isNetworkError {
			log.Printf("🚨 NETWORK ERROR (fasthttp failed to connect/dropped): %v | URL: %s", err, targetURL)
		} else if resp.StatusCode() == 502 {
			// Proxies often put the real error in the response body!
			log.Printf("🚨 PROXY RETURNED 502 HTTP STATUS. Body: %s | URL: %s", string(resp.Body()), targetURL)
		}
		// -----------------------------------

		log.Printf("❌ Request failed | provider=%s | url=%s | err=%v | status=%d",
			provider, targetURL, err, resp.StatusCode())

		if err != nil {
			if errors.Is(err, fasthttp.ErrTimeout) {
				finalStatusCode = 504
				sendJSONResponse(ctx, 504, false, "Upstream timeout", nil)
			} else {
				finalStatusCode = 502
				sendJSONResponse(ctx, 502, false, "Bad gateway", nil)
			}
			return
		}
		finalStatusCode = resp.StatusCode()
		sendJSONResponse(ctx, resp.StatusCode(), false, "Proxy error", map[string]interface{}{
			"upstreamStatus": resp.StatusCode(),
			"body":           resp.Body(),
		})
		return
	}

	updateInc := bson.M{"usedCredits": actualCost, "total": 1}
	if success {
		updateInc["success"] = 1
	} else if resp.StatusCode() == 429 {
		updateInc["quotaExceeded"] = 1
	} else {
		updateInc["fail"] = 1
	}

	res, err := collection.UpdateOne(
		context.Background(),
		bson.M{
			"apiKey": apiKey,
			"$expr": bson.M{
				"$lte": []interface{}{
					bson.M{"$add": []interface{}{"$usedCredits", actualCost}},
					totalEstimatedCredits,
				},
			},
		},
		bson.M{
			"$inc": updateInc,
			"$set": bson.M{"updatedAt": time.Now()},
		},
	)

	if err != nil {
		log.Println("⚠️ Mongo stats update failed:", err)
	}

	if res != nil && res.ModifiedCount == 0 {
		log.Printf("🚫 Quota exceeded AFTER request | key=%s | provider=%s | cost=%d", apiKey, provider, actualCost)
		collection.UpdateOne(context.Background(),
			bson.M{"apiKey": apiKey},
			bson.M{"$inc": bson.M{"usedCredits": actualCost, "quotaExceeded": 1, "total": 1}})

		finalStatusCode = 429
		sendJSONResponse(ctx, 429, false, "Quota exceeded", nil)
		return
	}

	log.Printf("✅ Success | provider=%s | status=%d | time=%v | cost=%d",
		provider, resp.StatusCode(), time.Since(start), actualCost)

	finalStatusCode = resp.StatusCode()

	resp.Header.VisitAll(func(k, v []byte) {
		ctx.Response.Header.SetBytesKV(k, v)
	})
	ctx.SetStatusCode(resp.StatusCode())
	ctx.SetBody(resp.Body())
}

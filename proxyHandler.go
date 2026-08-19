package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func secureFetchHandler(ctx *fasthttp.RequestCtx) {

	start := time.Now()

	// Single, anonymous key header. The caller never states which proxy
	// they want — that's resolved entirely from what's stored against
	// this key in Mongo (set by an admin).
	providedKey := string(ctx.Request.Header.Peek("api-key"))
	body := ctx.PostBody()
	var urlStr string
	var proxyParamsStr string

	if len(body) > 0 {
		var initialReq struct {
			URL         string          `json:"url"`
			ProxyParams json.RawMessage `json:"proxy_params"`
		}
		if err := json.Unmarshal(body, &initialReq); err == nil {
			urlStr = initialReq.URL
			if len(initialReq.ProxyParams) > 0 {
				proxyParamsStr = string(initialReq.ProxyParams)
			}
		}
	}

	var tokenID = primitive.NilObjectID

	if providedKey == "" {
		logRequest(ctx, "key missing", tokenID, urlStr, 401, start, 0)
		sendJSONResponse(ctx, 401, false, "Missing required header: api-key", nil)
		return
	}

	select {
	case <-ctx.Done():
		logRequest(ctx, providedKey, tokenID, urlStr, 499, start, 0)
		return
	default:
	}

	c, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Acquire quota semaphore
	select {
	case quotaSemaphore <- struct{}{}:
		defer func() { <-quotaSemaphore }()
	case <-ctx.Done():
		logRequest(ctx, providedKey, tokenID, urlStr, 499, start, 0)
		cancel()
		return
	case <-c.Done():
		logRequest(ctx, providedKey, tokenID, urlStr, 429, start, 0)
		sendJSONResponse(ctx, 429, false, "Server busy, please try again", nil)
		return
	}

	// Fetch user config from DB
	var aliasDoc ProxyConfig

	pipeline := mongo.Pipeline{
		bson.D{
			{Key: "$match", Value: bson.M{"apiKey": providedKey}},
		},
		bson.D{
			{Key: "$lookup", Value: bson.M{
				"from":         "tokens",
				"localField":   "tokenId",
				"foreignField": "_id",
				"as":           "tokenInfo",
			}},
		},
		bson.D{
			{Key: "$unwind", Value: "$tokenInfo"},
		},
		bson.D{
			{Key: "$project", Value: bson.M{
				"apiKey":                1,
				"proxyName":             1,
				"totalEstimatedCredits": 1,
				"usedCredits":           1,
				"status":                1,
				"token":                 "$tokenInfo.token",
				"tokenId":               1,
			}},
		},
	}

	cursor, err := collection.Aggregate(c, pipeline)
	if err != nil {
		log.Printf("❌ Aggregation failed: %v", err)
		logRequest(ctx, providedKey, tokenID, urlStr, 500, start, 0)
		sendJSONResponse(ctx, 500, false, "Internal server error", nil)
		return
	}

	var results []ProxyConfig

	if err := cursor.All(c, &results); err != nil {
		log.Printf("❌ Decode failed: %v", err)
		logRequest(ctx, providedKey, tokenID, urlStr, 500, start, 0)
		sendJSONResponse(ctx, 500, false, "Internal server error", nil)
		return
	}

	if len(results) == 0 {
		logRequest(ctx, providedKey, tokenID, urlStr, 401, start, 0)
		sendJSONResponse(ctx, 401, false, "Invalid API key", nil)
		return
	}

	aliasDoc = results[0]
	tokenID = aliasDoc.TokenID

	dbProxyName := strings.ToLower(aliasDoc.ProxyName)

	if aliasDoc.Token == "" {
		log.Printf("⚠️ No proxy token configured for api-key=%s", providedKey)
		logRequest(ctx, providedKey, tokenID, urlStr, 500, start, 0)
		sendJSONResponse(ctx, 500, false, "Proxy not configured. Contact support.", nil)
		return
	}

	// Pre-request quota check
	remaining := aliasDoc.TotalEstimatedCredits - aliasDoc.UsedCredits
	if remaining <= 0 {
		collection.UpdateOne(context.Background(),
			bson.M{"apiKey": providedKey},
			bson.M{"$inc": bson.M{"quotaExceeded": 1, "total": 1}})
		log.Printf("🚫 Quota exceeded BEFORE request | key=%s", providedKey)
		logRequest(ctx, providedKey, tokenID, urlStr, 429, start, 0)
		sendJSONResponse(ctx, 429, false, "Quota exceeded", map[string]interface{}{
			"usedCredits": aliasDoc.UsedCredits, "limit": aliasDoc.TotalEstimatedCredits, "remaining": 0,
		})
		return
	}

	if aliasDoc.Status != "active" {
		logRequest(ctx, providedKey, tokenID, urlStr, 403, start, 0)
		sendJSONResponse(ctx, 403, false, "key is inactive", map[string]interface{}{
			"status": aliasDoc.Status,
		})
		return
	}

	select {
	case <-ctx.Done():
		return
	default:
	}

	// The caller never knows which proxy is behind their key, so whatever
	// proxy_params they sent is always in Scrape.do's shape. Parse it as
	// that canonical format first...
	parsedCfg, err := parseProxyConfig("scrapedo", proxyParamsStr)
	if err != nil {
		log.Printf("❌ Config parse error: %v", err)
		logRequest(ctx, providedKey, tokenID, urlStr, 400, start, 0)
		sendJSONResponse(ctx, 400, false, err.Error(), nil)
		return
	}
	sdCfg, _ := parsedCfg.(*ScrapeDoConfig)
	if sdCfg == nil {
		sdCfg = &ScrapeDoConfig{}
	}

	// ...then dispatch to whichever proxy is actually configured for this
	// key, translating params only when the resolved proxy isn't Scrape.do.
	switch dbProxyName {
	case "scrapedo", "scrape.do", "scrape-do", "scrapedoproxy":
		fetchHandlerScrapeDo(ctx, providedKey, aliasDoc.TokenID, aliasDoc.Token, aliasDoc.TotalEstimatedCredits, sdCfg, start)

	case "scraperapi", "scraper_api", "scraper-api", "scraperapiproxy", "scraper api":
		apiCfg := convertScrapeDoToScraperAPI(sdCfg)
		fetchHandlerScraperAPI(ctx, providedKey, aliasDoc.TokenID, aliasDoc.Token, aliasDoc.TotalEstimatedCredits, apiCfg, start)

	default:
		log.Printf("❌ Unknown proxyName in DB: %s", aliasDoc.ProxyName)
		logRequest(ctx, providedKey, tokenID, urlStr, 500, start, 0)
		sendJSONResponse(ctx, 500, false, "Server misconfiguration: unknown proxy type", nil)
	}
}

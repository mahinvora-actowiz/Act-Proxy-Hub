package main

import (
	"encoding/json"
	"log"

	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func fetchHandlerActOne(ctx *fasthttp.RequestCtx, apiKey string, tokenID primitive.ObjectID, proxyToken string, totalEstimatedCredits int, start time.Time) {
	body := ctx.PostBody()
	if len(body) == 0 {
		sendJSONResponse(ctx, 400, false, "Request body is empty", nil)
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
		sendJSONResponse(ctx, 400, false, "Invalid JSON format", nil)
		return
	}

	if req.URL == "" {
		sendJSONResponse(ctx, 400, false, "Missing required field: url", nil)
		return
	}

	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "GET"
	}

	// 1. Build the map
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

	// 2. ✅ DO NOT CALL json.Marshal(actOneReq) HERE. 
	// Let executeRequest handle the marshaling so it sends a proper JSON object, not a string.

	outgoingHeaders := map[string]string{
		"X-API-Key": apiKey,
	}
	customHeadersStr, _ := json.Marshal(outgoingHeaders)

	// 3. ✅ PASS THE MAP DIRECTLY AS THE PAYLOAD
	executeRequest(
		ctx,
		apiKey,
		tokenID,
		"",
		ActOneEndpoint,
		req.URL,
		"POST",
		actOneReq,               // ✅ THIS MUST BE THE MAP, NOT string(actOneBody)
		string(customHeadersStr),
		totalEstimatedCredits,
		start,
		"actone",
	)
}
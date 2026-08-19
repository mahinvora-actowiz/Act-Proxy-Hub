package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/bcrypt"
)

func getClientIP(ctx *fasthttp.RequestCtx) string {
	if ip := string(ctx.Request.Header.Peek("X-Forwarded-For")); ip != "" {
		parts := strings.Split(ip, ",")
		return strings.TrimSpace(parts[0])
	}

	if ip := string(ctx.Request.Header.Peek("X-Real-IP")); ip != "" {
		return ip
	}

	return ctx.RemoteIP().String()
}

// buildScraperAPIProxyOptions builds the options string for ScraperAPI proxy mode
func buildScraperAPIProxyOptions(cfg *ScraperAPIConfig) string {
	var opts []string
	opts = append(opts, "scraperapi")

	if cfg.CountryCode != "" {
		opts = append(opts, fmt.Sprintf("country_code=%s", cfg.CountryCode))
	}
	if cfg.Zip != "" {
		opts = append(opts, fmt.Sprintf("zip=%s", cfg.Zip))
	}
	if cfg.Premium != nil && *cfg.Premium {
		opts = append(opts, "premium=true")
	}
	if cfg.UltraPremium != nil && *cfg.UltraPremium {
		opts = append(opts, "ultra_premium=true")
	}
	if cfg.Render != nil && *cfg.Render {
		opts = append(opts, "render=true")
	}
	if cfg.SessionNumber != nil {
		opts = append(opts, fmt.Sprintf("session_number=%d", *cfg.SessionNumber))
	}
	if cfg.KeepHeaders != nil && *cfg.KeepHeaders {
		opts = append(opts, "keep_headers=true")
	}
	if cfg.DeviceType != "" {
		opts = append(opts, fmt.Sprintf("device_type=%s", cfg.DeviceType))
	}
	if cfg.Screenshot != nil && *cfg.Screenshot {
		opts = append(opts, "screenshot=true")
	}
	if cfg.WaitForSelector != "" {
		opts = append(opts, fmt.Sprintf("wait_for_selector=%s", url.QueryEscape(cfg.WaitForSelector)))
	}
	if cfg.CustomWait > 0 {
		opts = append(opts, fmt.Sprintf("wait=%d", cfg.CustomWait))
	}
	if cfg.OutputFormat != "" {
		opts = append(opts, fmt.Sprintf("output_format=%s", cfg.OutputFormat))
	}
	if cfg.AutoParse != nil && *cfg.AutoParse {
		opts = append(opts, "autoparse=true")
	}
	if cfg.Timeout > 0 {
		opts = append(opts, fmt.Sprintf("timeout=%d", cfg.Timeout))
	}
	if cfg.MaxRetries > 0 {
		opts = append(opts, fmt.Sprintf("max_retry_attempts=%d", cfg.MaxRetries))
	}
	if cfg.FollowRedirect != nil && !*cfg.FollowRedirect {
		opts = append(opts, "follow_redirect=false")
	}

	return strings.Join(opts, ".")
}

func setBool(params url.Values, key string, val *bool) {
	if val != nil {
		if *val {
			params.Set(key, "true")
		} else {
			params.Set(key, "false")
		}
	}
}

func setString(params url.Values, key, val string) {
	if val != "" {
		params.Set(key, val)
	}
}

func setInt(params url.Values, key string, val int) {
	if val > 0 {
		params.Set(key, strconv.Itoa(val))
	}
}

func setIntPtr(params url.Values, key string, val *int) {
	if val != nil {
		params.Set(key, strconv.Itoa(*val))
	}
}

func ProcessJSONToEncodedCookies(jsonRaw string) (string, error) {
	if jsonRaw == "" {
		return "", nil
	}

	var cookieMap map[string]interface{}
	err := json.Unmarshal([]byte(jsonRaw), &cookieMap)
	if err != nil {
		return "", fmt.Errorf("invalid JSON format: %v", err)
	}

	if len(cookieMap) == 0 {
		return "", fmt.Errorf("cookie JSON cannot be empty")
	}

	var pairs []string
	for k, v := range cookieMap {
		valStr := fmt.Sprint(v)
		if k == "" || valStr == "" {
			return "", fmt.Errorf("cookie keys and values cannot be empty")
		}
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
	}

	rawString := strings.Join(pairs, "; ")
	return url.QueryEscape(rawString), nil
}

func HashPassword(password string) (string, error) {

	bytes, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		14,
	)

	return string(bytes), err
}

func CheckPassword(
	hashedPassword string,
	password string,
) bool {

	err := bcrypt.CompareHashAndPassword(
		[]byte(hashedPassword),
		[]byte(password),
	)

	return err == nil
}

func checkAdminKey(ctx *fasthttp.RequestCtx) bool {
	adminKey := string(ctx.Request.Header.Peek("admin-key"))
	if adminKey != "8f7c9c6a-4b9d-4f8a-a0b7-91f2d3e4c5b6" {
		sendJSONResponse(ctx, 401, false, "Unauthorized", nil)
		return false
	}
	return true
}

func checkUserKey(ctx *fasthttp.RequestCtx) bool {
	apiKey := string(ctx.Request.Header.Peek("api-key"))
	if apiKey != "2d1e7f90-8c44-41c9-b6f2-5a8d9e0f7b12" {
		sendJSONResponse(ctx, 401, false, "Unauthorized", nil)
		return false
	}
	return true
}

// sendJSONResponse sends a consistent JSON response
func sendJSONResponse(ctx *fasthttp.RequestCtx, statusCode int, success bool, message string, data interface{}) {
	response := map[string]interface{}{
		"success": success,
		"message": message,
	}

	if data != nil {
		response["data"] = data
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		ctx.SetStatusCode(500)
		ctx.SetContentType("application/json")
		ctx.SetBodyString(`{"success":false,"message":"Internal server error"}`)
		return
	}

	ctx.SetContentType("application/json")
	ctx.SetStatusCode(statusCode)
	ctx.SetBody(jsonData)
}

// sendPaginatedJSONResponse sends a consistent JSON response with pagination
func sendPaginatedJSONResponse(ctx *fasthttp.RequestCtx, statusCode int, success bool, message string, data interface{}, page, limit, total int) {
	response := map[string]interface{}{
		"success":    success,
		"message":    message,
		"data":       data,
		"page":       page,
		"limit":      limit,
		"total":      total,
		"totalPages": int(math.Ceil(float64(total) / float64(limit))),
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		ctx.SetStatusCode(500)
		ctx.SetContentType("application/json")
		ctx.SetBodyString(`{"success":false,"message":"Internal server error"}`)
		return
	}

	ctx.SetContentType("application/json")
	ctx.SetStatusCode(statusCode)
	ctx.SetBody(jsonData)
}

func parseProxyConfig(proxyName string, proxiesStr string) (interface{}, error) {
	if proxiesStr == "" {
		return nil, nil
	}

	switch strings.ToLower(proxyName) {
	case "scraperapi", "scraper_api":
		var cfg ScraperAPIConfig
		if err := json.Unmarshal([]byte(proxiesStr), &cfg); err != nil {
			return nil, fmt.Errorf("invalid ScraperAPI config: %w", err)
		}
		if cfg.Premium != nil && *cfg.Premium && cfg.UltraPremium != nil && *cfg.UltraPremium {
			return nil, errors.New("premium and ultra_premium cannot be used together")
		}
		if cfg.WaitForSelector != "" && (cfg.Render == nil || !*cfg.Render) {
			log.Printf("⚠️ wait_for_selector requires render=true, auto-enabling render")
			render := true
			cfg.Render = &render
		}
		return &cfg, nil
	case "scrapedo", "scrape.do", "":
		var cfg ScrapeDoConfig
		if err := json.Unmarshal([]byte(proxiesStr), &cfg); err != nil {
			return nil, fmt.Errorf("invalid Scrape.do config: %w", err)
		}
		return &cfg, nil
	default:
		return nil, fmt.Errorf("unsupported proxyName: %s", proxyName)
	}
}

// --- Helper functions for safe, clean type extraction ---
func getBoolPtr(m map[string]interface{}, key string) *bool {
	if v, ok := m[key].(bool); ok {
		return &v
	}
	return nil
}

func getStringVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getIntPtr(m map[string]interface{}, key string) *int {
	if v, ok := m[key].(float64); ok {
		val := int(v)
		return &val
	}
	return nil
}

// convertScrapeDoToScraperAPI translates a canonical Scrape.do-shaped config
// (the only format callers ever send, since they don't know which proxy is
// actually behind their key) into the closest equivalent ScraperAPIConfig.
// Not every Scrape.do option has a ScraperAPI equivalent — those are dropped
// silently since ScraperAPI has no matching feature.
func convertScrapeDoToScraperAPI(src *ScrapeDoConfig) *ScraperAPIConfig {
	if src == nil {
		return &ScraperAPIConfig{}
	}

	dst := &ScraperAPIConfig{}

	// Booleans
	if src.Render != nil {
		dst.Render = src.Render
	}
	if src.ScreenShot != nil {
		dst.Screenshot = src.ScreenShot
	} else if src.FullScreenShot != nil {
		dst.Screenshot = src.FullScreenShot
	}
	if src.Super != nil && *src.Super {
		premium := true
		dst.Premium = &premium
	}
	if src.ReturnJSON != nil {
		dst.AutoParse = src.ReturnJSON
	}
	if src.ForwardHeaders != nil {
		dst.KeepHeaders = src.ForwardHeaders
	} else if src.CustomHeaders != nil {
		dst.KeepHeaders = src.CustomHeaders
	}
	if src.DisableRedirection != nil {
		follow := !*src.DisableRedirection
		dst.FollowRedirect = &follow
	}

	// Strings
	if src.GeoCode != "" {
		dst.CountryCode = src.GeoCode
	} else if src.RegionalGeoCode != "" {
		dst.CountryCode = src.RegionalGeoCode
	}
	if src.WaitSelector != "" {
		dst.WaitForSelector = src.WaitSelector
		// ScraperAPI requires render=true for wait_for_selector to work
		if dst.Render == nil || !*dst.Render {
			render := true
			dst.Render = &render
		}
	}
	if src.Device != "" {
		dst.DeviceType = src.Device
	}
	if src.Output != "" {
		dst.OutputFormat = src.Output
	}

	// Numbers
	if src.SessionId != nil {
		dst.SessionNumber = src.SessionId
	}
	if src.Timeout > 0 {
		dst.Timeout = src.Timeout
	}
	if src.CustomWait > 0 {
		dst.CustomWait = src.CustomWait
	}

	return dst
}

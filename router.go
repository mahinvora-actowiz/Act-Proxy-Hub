package main

import (
	"log"

	"github.com/valyala/fasthttp"
)

var maxConcurrent = 5000
var sem = make(chan struct{}, maxConcurrent)

var quotaSemaphore = make(chan struct{}, 500)

func router(ctx *fasthttp.RequestCtx) {

	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()

		switch string(ctx.Path()) {
		case "/fetch":
			secureFetchHandler(ctx)
		default:
			ctx.SetStatusCode(404)
			ctx.SetBodyString("Not Found")
		}

	default:
		ctx.SetStatusCode(429)
		ctx.SetBodyString("Too Many Requests")

		log.Println("🚫 429 Too Many Requests | IP =", ctx.RemoteIP())
	}
}

package main

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/valyala/fasthttp"
)

var usageCounter = make(map[string]int)
var usageLock sync.Mutex

func main() {

	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	// Read environment variables
	port := os.Getenv("PORT")
	mongoURI := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")

	// Validate required variables
	if port == "" {
		port = "8080"
	}

	if mongoURI == "" {
		log.Fatal("MONGO_URI is not set")
	}

	if dbName == "" {
		log.Fatal("DB_NAME is not set")
	}

	log.Println("Starting server...")
	log.Println("Port:", port)
	log.Println("Database:", dbName)

	// Initialize MongoDB
	initMongo()

	server := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {

			enableCORS(ctx)

			if string(ctx.Method()) == "OPTIONS" {
				return
			}

			router(ctx)
		},

		MaxConnsPerIP:      5000,
		MaxRequestsPerConn: 1000,

		ReadTimeout:  300 * time.Second,
		WriteTimeout: 500 * time.Second,

		MaxRequestBodySize: 10 * 1024 * 1024,

		DisableKeepalive: false,

		ReadBufferSize:  65536,
		WriteBufferSize: 65536,
	}

	log.Printf("High Performance Fetch Server Running on :%s", port)

	log.Fatal(server.ListenAndServe("0.0.0.0:" + port))
}

func enableCORS(ctx *fasthttp.RequestCtx) {

	ctx.Response.Header.Set(
		"Access-Control-Allow-Origin",
		"*",
	)

	ctx.Response.Header.Set(
		"Access-Control-Allow-Methods",
		"GET, POST, PUT, DELETE, OPTIONS",
	)

	ctx.Response.Header.Set(
		"Access-Control-Allow-Headers",
		"Content-Type, Authorization, api-key, admin-key",
	)

	if string(ctx.Method()) == "OPTIONS" {
		ctx.SetStatusCode(fasthttp.StatusOK)
		return
	}
}

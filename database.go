package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const systemTimeOffset = 11*time.Hour + 30*time.Minute

func getActualTime() time.Time {
	return time.Now().Add(systemTimeOffset)
}

func getDailyLogCollection() *mongo.Collection {
	dateStr := getActualTime().Format("2006_01_02")

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		log.Fatal("DB_NAME is not set")
	}

	return mongoClient.Database(dbName).Collection("logs_" + dateStr)
}

// addDomainToAlias adds a distinct domain to the domains array in the aliases collection
func addDomainToAlias(collection *mongo.Collection, apiKey, domain string) error {
	if domain == "" || apiKey == "" {
		return nil
	}

	_, err := collection.UpdateOne(
		context.Background(),
		bson.M{"apiKey": apiKey},
		bson.M{
			"$addToSet": bson.M{
				"domains": domain,
			},
			"$set": bson.M{
				"updatedAt": time.Now(),
			},
		},
	)

	return err
}

func logRequest(
	ctx *fasthttp.RequestCtx,
	apiKey string,
	tokenID primitive.ObjectID,
	targetURL string,
	statusCode int,
	start time.Time,
	actualCost int,
) {
	domain := "N/A"

	if targetURL != "" {
		if parsedURL, err := url.Parse(targetURL); err == nil && parsedURL.Hostname() != "" {
			domain = parsedURL.Hostname()
		}
	}

	if apiKey == "" {
		apiKey = "unknown"
	}

	logDoc := RequestLog{
		Key:          apiKey,
		TokenID:      tokenID,
		Domain:       domain,
		IP:           ctx.RemoteIP().String(),
		StatusCode:   statusCode,
		ResponseTime: time.Since(start).Milliseconds(),
		CreditUsed:   actualCost,
		CreatedAt:    time.Now(),
	}

	go func() {
		logCollection := getDailyLogCollection()

		insertCtx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		if _, err := logCollection.InsertOne(insertCtx, logDoc); err != nil {
			log.Printf("⚠️ Failed to insert request log: %v", err)
		}
	}()
}

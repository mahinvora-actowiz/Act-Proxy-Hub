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

const systemTimeOffset = 0*time.Hour + 0*time.Minute

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
	originalTargetURL string,
	statusCode int,
	start time.Time,
	actualCost int,
	errorMessage string,
	errorSource string,
) {
	domain := "N/A"

	if originalTargetURL != "" {
		if parsedURL, err := url.Parse(originalTargetURL); err == nil && parsedURL.Hostname() != "" {
			domain = parsedURL.Hostname()
		}
	}

	if apiKey == "" {
		apiKey = "unknown"
	}

	// Use getActualTime() for consistency with your systemTimeOffset logic
	currentTime := time.Now()
	proxyProvider, _ := ctx.UserValue("proxyProvider").(string)

	logDoc := RequestLog{
		Key:           apiKey,
		ProxyProvider: proxyProvider,
		TokenID:       tokenID,
		Domain:        domain,
		IP:            ctx.RemoteIP().String(),
		StatusCode:    statusCode,
		ResponseTime:  time.Since(start).Milliseconds(),
		CreditUsed:    actualCost,
		CreatedAt:     currentTime,
		ErrorMessage:  errorMessage,
		ErrorSource:   errorSource,
	}

	go func() {
		logCollection := getDailyLogCollection()

		insertCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		if _, err := logCollection.InsertOne(insertCtx, logDoc); err != nil {
			log.Printf("⚠️ Failed to insert request log: %v", err)
		}
	}()
}

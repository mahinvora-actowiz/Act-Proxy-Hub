package main

import (
	"context"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoClient *mongo.Client
var collection *mongo.Collection
var tokensCollection *mongo.Collection

func initMongo() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoURI := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")

	if mongoURI == "" {
		log.Fatal("MONGO_URI is not set")
	}

	if dbName == "" {
		log.Fatal("DB_NAME is not set")
	}

	client, err := mongo.Connect(
		ctx,
		options.Client().ApplyURI(mongoURI),
	)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Verify connection
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}

	mongoClient = client

	db := client.Database(dbName)

	collection = db.Collection("aliases")
	tokensCollection = db.Collection("tokens")

	log.Printf("MongoDB connected successfully: database=%s", dbName)
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
	"sync"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Coordinates represents the embedded document for XY field
type Coordinates struct {
	X  float64 `json:"x" bson:"x"`
	Y float64 `json:"y" bson:"y"`
}

// MapLocation represents your data structure
type MapLocation struct {
	ID string `json:"id" bson:"_id"`
	Location string      `json:"location" bson:"location"`
	XY       Coordinates `json:"xy" bson:"xy"`
}

var (
	client     *mongo.Client
	collection *mongo.Collection
	data       []MapLocation
	cache      struct {
		data []MapLocation
	}
	cacheMutex sync.Mutex
)

func initMongoDB() error {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		return err
	}

	// Parse the connection string
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		return fmt.Errorf("MONGO_URI environment variable is not set")
	}

	clientOptions := options.Client().ApplyURI(uri).SetMaxPoolSize(10) // Adjust the pool size as needed

	// Connect to MongoDB
	client, err = mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		return err
	}

	collection = client.Database("soulforged-db").Collection("maplocations")

	// Check the connection
	err = client.Ping(context.Background(), nil)
	if err != nil {
		return err
	}

	fmt.Println("Connected to MongoDB!")

	return nil
}

func getMapDataHandler(w http.ResponseWriter, r *http.Request) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")

	if len(cache.data) == 0 {
		// Fetch data from MongoDB
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		cursor, err := collection.Find(ctx, bson.D{})
		if err != nil {
			http.Error(w, "Failed to fetch map data from MongoDB", http.StatusInternalServerError)
			return
		}
		defer cursor.Close(ctx)

		if err := cursor.All(context.Background(), &data); err != nil {
			http.Error(w, "Failed to decode map data", http.StatusInternalServerError)
			return
		}

		// Update cache
		cache.data = data
		fmt.Println("Cache updated")
	}
	
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(cache.data); err != nil {
		http.Error(w, "Failed to encode map data as JSON", http.StatusInternalServerError)
		return
	}
}

func updateCacheAsync(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cacheMutex.Lock()
			// Fetch data from MongoDB and update cache
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cursor, err := collection.Find(ctx, bson.D{})
			if err != nil {
				fmt.Println("Error fetching data from MongoDB:", err)
				cacheMutex.Unlock()
				continue
			}
			defer cursor.Close(ctx)

			if err := cursor.All(context.Background(), &data); err != nil {
				fmt.Println("Error decoding map data:", err)
				cacheMutex.Unlock()
				continue
			}

			// Update cache
			cache.data = data
			fmt.Println("Cache updated")
			cacheMutex.Unlock()
		}
	}
}

func main() {
	// Initialize the router and MongoDB
	err := initMongoDB()
	if err != nil {
		fmt.Println("Error initializing MongoDB:", err)
		return
	}

	// Set your update interval (e.g., every 5 minutes)
	updateInterval := 20 * time.Second

	// Start updating the cache asynchronously
	go updateCacheAsync(updateInterval)

	// Register the handler
	http.HandleFunc("/api/map", getMapDataHandler)

	// Set your port here
	port := 8080

	// Start the server
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

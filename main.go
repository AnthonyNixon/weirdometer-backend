package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	firestore "cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

type Rating struct {
	UserID  string `json:"userId"`
	ImageID int    `json:"imageId"`
	Rating  int    `json:"rating"`
}

type Image struct {
	ImageID       int      `json:"imageId"`
	ImageUrl      string   `json:"imageUrl"`
	TotalRating   int      `json:"totalRating"`
	RatingCount   int      `json:"ratingCount"`
	AverageRating float64  `json:"averageRating"`
	RatedBy       []string `json:"ratedBy"`
}

var (
	images = []Image{
		{ImageID: 1, ImageUrl: "path/to/your/image1.jpg"},
		{ImageID: 2, ImageUrl: "path/to/your/image2.jpg"},
	}
	imageMap = make(map[int]*Image)
	mu       sync.Mutex
	client   *firestore.Client
	ctx      context.Context
)

func init() {
	for i := range images {
		imageMap[images[i].ImageID] = &images[i]
	}
	ctx = context.Background()
	var err error
	client, err = firestore.NewClient(ctx, "YOUR_PROJECT_ID", option.WithCredentialsFile("path/to/serviceAccountKey.json"))
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
}

func rateHandler(w http.ResponseWriter, r *http.Request) {
	var rating Rating
	if err := json.NewDecoder(r.Body).Decode(&rating); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if image, ok := imageMap[rating.ImageID]; ok {
		// Check if the user has already rated this image
		for _, userID := range image.RatedBy {
			if userID == rating.UserID {
				http.Error(w, "User has already rated this image", http.StatusConflict)
				return
			}
		}
		image.TotalRating += rating.Rating
		image.RatingCount++
		image.AverageRating = float64(image.TotalRating) / float64(image.RatingCount)
		image.RatedBy = append(image.RatedBy, rating.UserID)

		// Store the rating in Firestore
		_, err := client.Collection("ratings").Doc(rating.UserID).Set(ctx, map[string]interface{}{
			"imageId": rating.ImageID,
			"rating":  rating.Rating,
		})
		if err != nil {
			http.Error(w, "Failed to store rating", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(image)
		return
	}
	http.Error(w, "Image not found", http.StatusNotFound)
}

func nextImageHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		http.Error(w, "userId query parameter is required", http.StatusBadRequest)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, image := range images {
		rated := false
		for _, userIDRated := range image.RatedBy {
			if userIDRated == userID {
				rated = true
				break
			}
		}
		if !rated {
			json.NewEncoder(w).Encode(image)
			return
		}
	}
	http.Error(w, "No more images to rate", http.StatusNotFound)
}

func leaderboardHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	json.NewEncoder(w).Encode(images)
}

func main() {
	http.HandleFunc("/rate", rateHandler)
	http.HandleFunc("/next-image", nextImageHandler)
	http.HandleFunc("/leaderboard", leaderboardHandler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

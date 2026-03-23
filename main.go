package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync" // This imports the Mutex package
	"time"
)

type URLData struct {
	OriginalURL string `json:"original_url"`
	ShortCode   string `json:"short_code"`
	Clicks      int    `json:"clicks"`
}

// 1. Our In-Memory Database and Mutex
var (
	db    = make(map[string]URLData)
	mutex sync.RWMutex // Protects 'db' from Race Conditions
)

func generateShortCode() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	code := make([]byte, 6)
	for i := range code {
		code[i] = charset[rand.Intn(len(charset))]
	}
	return string(code)
}

func handleShorten(w http.ResponseWriter, r *http.Request) {
	// 1. Add CORS headers so React can talk to Go
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// 2. Handle the "Preflight" OPTIONS request sent by browsers
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var reqBody struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	shortCode := generateShortCode()

	mutex.Lock()
	db[shortCode] = URLData{
		OriginalURL: reqBody.URL,
		ShortCode:   shortCode,
		Clicks:      0,
	}
	mutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"short_url": fmt.Sprintf("http://localhost:8080/%s", shortCode),
	})
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path == "/favicon.ico" {
		http.NotFound(w, r)
		return
	}

	shortCode := strings.Trim(r.URL.Path, "/")

	// 3. Lock for Reading
	mutex.RLock()
	urlData, exists := db[shortCode]
	mutex.RUnlock()

	if !exists {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	// 4. Lock for Writing to update the analytics
	mutex.Lock()
	urlData.Clicks++
	db[shortCode] = urlData
	mutex.Unlock()

	http.Redirect(w, r, urlData.OriginalURL, http.StatusFound)
}

func main() {
	http.HandleFunc("/shorten", handleShorten)
	http.HandleFunc("/", handleRedirect)

	// Fetch the port provided by Render, or default to 8080 for local testing
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Server is running on port " + port + "...")
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
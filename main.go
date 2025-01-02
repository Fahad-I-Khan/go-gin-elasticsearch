// main.go
package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esutil"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Blog represents the blog model
type Blog struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	Title     string    `gorm:"type:varchar(255);not null"`
	Content   string    `gorm:"type:text;not null"`
	Author    string    `gorm:"type:varchar(100);not null"`
	Category  string    `gorm:"type:varchar(50);not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

var (
	db     *gorm.DB
	es     *elasticsearch.Client
	router *gin.Engine
)

func initPostgres() {
	var err error
	dsn := "host=postgres user=postgres password=postgres dbname=blog port=5432 sslmode=disable"
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	if err := db.AutoMigrate(&Blog{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Connected to PostgreSQL and migrated schema.")
}

func initElasticsearch() {
	var err error

	for i := 0; i < 5; i++ { // Retry up to 5 times
		es, err = elasticsearch.NewDefaultClient()
		if err != nil {
			log.Printf("Attempt %d: Failed to create Elasticsearch client: %v", i+1, err)
			time.Sleep(5 * time.Second)
			continue
		}

		res, err := es.Ping()
		if err != nil || res.IsError() {
			log.Printf("Attempt %d: Failed to ping Elasticsearch: %v", i+1, err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("Connected to Elasticsearch.")
		return
	}

	log.Fatalf("Failed to connect to Elasticsearch after multiple attempts: %v", err)
}

func createBlog(c *gin.Context) {
	var blog Blog
	if err := c.ShouldBindJSON(&blog); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}

	if err := db.Create(&blog).Error; err != nil {
		c.JSON(500, gin.H{"error": "Failed to create blog"})
		return
	}

	doc := map[string]interface{}{
		"title":     blog.Title,
		"content":   blog.Content,
		"author":    blog.Author,
		"category":  blog.Category,
		"createdAt": blog.CreatedAt.Format(time.RFC3339),
	}

	// Convert blog.ID (uint) to string for Elasticsearch
    idString := strconv.FormatUint(uint64(blog.ID), 10) // Convert uint to string

	_, err := es.Index(
		"blogs",
		esutil.NewJSONReader(doc),
		es.Index.WithDocumentID(idString),
		es.Index.WithRefresh("true"),
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to index blog in Elasticsearch"})
		return
	}

	c.JSON(201, blog)
}

func getBlog(c *gin.Context) {
	id := c.Param("id")
	var blog Blog
	if err := db.First(&blog, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "Blog not found"})
		return
	}
	c.JSON(200, blog)
}

func updateBlog(c *gin.Context) {
	id := c.Param("id")
	var blog Blog

	// Fetch the existing blog
	if err := db.First(&blog, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "Blog not found"})
		return
	}

	// Parse the incoming request for the updated blog data
	var updatedBlog Blog
	if err := c.ShouldBindJSON(&updatedBlog); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}

	// Delete the old document in Elasticsearch
	_, err := es.Delete(
		"blogs",
		id, // Document ID matches the blog's ID
		es.Delete.WithRefresh("true"),
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete previous blog version from Elasticsearch"})
		return
	}

	// Update the blog in the database
	blog.Title = updatedBlog.Title
	blog.Content = updatedBlog.Content
	blog.Author = updatedBlog.Author
	blog.Category = updatedBlog.Category

	if err := db.Save(&blog).Error; err != nil {
		c.JSON(500, gin.H{"error": "Failed to update blog in the database"})
		return
	}

	// Reindex the updated blog in Elasticsearch
	doc := map[string]interface{}{
		"title":     blog.Title,
		"content":   blog.Content,
		"author":    blog.Author,
		"category":  blog.Category,
		"createdAt": blog.CreatedAt.Format(time.RFC3339),
	}

	// Convert blog.ID (uint) to string before passing it to es.Index
	idString := strconv.FormatUint(uint64(blog.ID), 10) // Convert uint to string

	_, err = es.Index(
		"blogs",
		esutil.NewJSONReader(doc),
		es.Index.WithDocumentID(idString), // Use the blog's ID for indexing
		es.Index.WithRefresh("true"),
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to index updated blog in Elasticsearch"})
		return
	}

	c.JSON(200, gin.H{"message": "Blog updated successfully", "blog": blog})
}



func deleteBlog(c *gin.Context) {
	id := c.Param("id")

	// Delete the blog from the database
	if err := db.Delete(&Blog{}, id).Error; err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete blog from the database"})
		return
	}

	// Delete the blog from Elasticsearch
	_, err := es.Delete(
		"blogs",
		id, // Document ID matches the blog's ID
		es.Delete.WithRefresh("true"),
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete blog from Elasticsearch"})
		return
	}

	c.JSON(200, gin.H{"message": "Blog deleted successfully"})
}



func searchBlog(c *gin.Context) {
	// Extract query parameters from the request
	query := c.Query("query")                  // Search term
	startDate := c.Query("startDate")          // Start date for filtering
	endDate := c.Query("endDate")              // End date for filtering
	page, err := strconv.Atoi(c.Query("page")) // Pagination: Page number
	if err != nil || page < 1 {
		page = 1 // Default to page 1 if invalid
	}
	size, err := strconv.Atoi(c.Query("size")) // Pagination: Results per page
	if err != nil || size < 1 {
		size = 10 // Default to 10 results per page if invalid
	}

	// Construct Elasticsearch query
	esQuery := map[string]interface{}{
		"from": (page - 1) * size, // Offset calculation
		"size": size,              // Number of results per page
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"createdAt": map[string]string{
								"gte": startDate, // Greater than or equal to start date
								"lte": endDate,   // Less than or equal to end date
							},
						},
					},
				},
				"must": []map[string]interface{}{
					{
						"multi_match": map[string]interface{}{
							"fields": []string{"title", "content"}, // Fields to search in
							"query":  query,                        // Search term
						},
					},
				},
			},
		},
	}

	// Convert the query to JSON
	body, err := json.Marshal(esQuery)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build query"})
		return
	}

	// Log the constructed query for debugging
	log.Printf("Search Query: %s\n", string(body))

	// Send the query to Elasticsearch
	esURL := "http://elasticsearch:9200/blogs/_search" // Elasticsearch endpoint
	resp, err := http.Post(esURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Elasticsearch"})
		return
	}
	defer resp.Body.Close()

	// Read the response from Elasticsearch
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read Elasticsearch response"})
		return
	}

	// Log Elasticsearch's response for debugging
	log.Printf("Elasticsearch Response: %s\n", string(respBody))

	// Parse and return the response
	var esResponse map[string]interface{}
	err = json.Unmarshal(respBody, &esResponse)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse Elasticsearch response"})
		return
	}

	// Return the Elasticsearch response to the client
	c.JSON(http.StatusOK, esResponse)
}

func main() {
	initPostgres()
	initElasticsearch()

	router = gin.Default()
	router.Use(cors.Default())

	router.POST("/blogs", createBlog)
	router.GET("/blogs/:id", getBlog)
	router.PUT("/blogs/:id", updateBlog)
	router.DELETE("/blogs/:id", deleteBlog)
	router.GET("/blogs/search", searchBlog)

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

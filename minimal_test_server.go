package main

import (
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.New()
	r.Use(gin.Recovery())

	// Add debug middleware to trace requests
	r.Use(func(c *gin.Context) {
		log.Printf("DEBUG: Incoming request: %s %s", c.Request.Method, c.Request.URL.Path)
		c.Next()
		log.Printf("DEBUG: Request completed with status: %d", c.Writer.Status())
	})

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	r.GET("/ping", func(c *gin.Context) {
		log.Printf("DEBUG: Ping handler called")
		c.JSON(200, gin.H{"message": "pong"})
	})

	r.POST("/adapters", func(c *gin.Context) {
		log.Printf("DEBUG: Create adapter called")
		c.JSON(200, gin.H{"status": "ok"})
	})

	log.Println("Minimal test server listening on :8914")
	log.Fatal(r.Run(":8914"))
}

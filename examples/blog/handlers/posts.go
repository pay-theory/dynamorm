package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"

	"github.com/example/dynamorm"
	"github.com/example/dynamorm/examples/blog/models"
)

// PostHandler handles blog post operations
type PostHandler struct {
	db *dynamorm.DB
}

// NewPostHandler creates a new post handler
func NewPostHandler() (*PostHandler, error) {
	db, err := dynamorm.New(
		dynamorm.WithLambdaOptimization(),
		dynamorm.WithConnectionPool(10),
		dynamorm.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&models.Post{})
	db.Model(&models.Author{})
	db.Model(&models.Category{})
	db.Model(&models.Tag{})
	db.Model(&models.SearchIndex{})
	db.Model(&models.Analytics{})

	return &PostHandler{db: db}, nil
}

// HandleRequest routes requests to appropriate handlers
func (h *PostHandler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch request.HTTPMethod {
	case "GET":
		if request.PathParameters["slug"] != "" {
			return h.getPostBySlug(ctx, request)
		}
		return h.listPosts(ctx, request)
	case "POST":
		return h.createPost(ctx, request)
	case "PUT":
		return h.updatePost(ctx, request)
	case "DELETE":
		return h.deletePost(ctx, request)
	default:
		return errorResponse(http.StatusMethodNotAllowed, "Method not allowed"), nil
	}
}

// listPosts returns paginated list of posts
func (h *PostHandler) listPosts(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Parse query parameters
	status := request.QueryStringParameters["status"]
	if status == "" {
		status = models.PostStatusPublished
	}

	limit, _ := strconv.Atoi(request.QueryStringParameters["limit"])
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	cursor := request.QueryStringParameters["cursor"]
	authorID := request.QueryStringParameters["author_id"]
	categoryID := request.QueryStringParameters["category_id"]
	tag := request.QueryStringParameters["tag"]

	// Build query
	query := h.db.Model(&models.Post{}).
		Index("gsi-status-date").
		Where("Status", "=", status).
		OrderBy("PublishedAt", "DESC").
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Apply filters
	if authorID != "" {
		// Use author index instead
		query = h.db.Model(&models.Post{}).
			Index("gsi-author").
			Where("AuthorID", "=", authorID).
			Filter("Status = :status", dynamorm.Param("status", status)).
			Limit(limit)
	} else if categoryID != "" {
		query = query.Filter("CategoryID = :cat", dynamorm.Param("cat", categoryID))
	}

	if tag != "" {
		query = query.Filter("contains(Tags, :tag)", dynamorm.Param("tag", tag))
	}

	// Execute query
	var posts []*models.Post
	nextCursor, err := query.All(&posts)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch posts"), nil
	}

	// Enrich posts with author info (in production, consider caching)
	authorIDs := make([]string, 0, len(posts))
	authorMap := make(map[string]*models.Author)

	for _, post := range posts {
		authorIDs = append(authorIDs, post.AuthorID)
	}

	// Batch get authors
	if len(authorIDs) > 0 {
		var authors []*models.Author
		err = h.db.Model(&models.Author{}).
			Where("ID", "in", authorIDs).
			All(&authors)

		for _, author := range authors {
			authorMap[author.ID] = author
		}
	}

	// Build response
	type enrichedPost struct {
		*models.Post
		Author *models.Author `json:"author,omitempty"`
	}

	enrichedPosts := make([]enrichedPost, len(posts))
	for i, post := range posts {
		enrichedPosts[i] = enrichedPost{
			Post:   post,
			Author: authorMap[post.AuthorID],
		}
	}

	response := map[string]interface{}{
		"posts":       enrichedPosts,
		"next_cursor": nextCursor,
		"has_more":    nextCursor != "",
		"limit":       limit,
	}

	return successResponse(http.StatusOK, response), nil
}

// getPostBySlug retrieves a post by its slug
func (h *PostHandler) getPostBySlug(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	slug := request.PathParameters["slug"]
	if slug == "" {
		return errorResponse(http.StatusBadRequest, "Slug is required"), nil
	}

	// Get post by slug
	var post models.Post
	err := h.db.Model(&models.Post{}).
		Index("gsi-slug").
		Where("Slug", "=", slug).
		First(&post)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			return errorResponse(http.StatusNotFound, "Post not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch post"), nil
	}

	// Only return published posts to public
	if post.Status != models.PostStatusPublished && !isAuthorized(request) {
		return errorResponse(http.StatusNotFound, "Post not found"), nil
	}

	// Increment view count atomically
	go h.incrementViewCount(post.ID, getSessionID(request))

	// Get author
	var author models.Author
	_ = h.db.Model(&models.Author{}).
		Where("ID", "=", post.AuthorID).
		First(&author)

	// Get category
	var category models.Category
	if post.CategoryID != "" {
		_ = h.db.Model(&models.Category{}).
			Where("ID", "=", post.CategoryID).
			First(&category)
	}

	// Build response
	response := map[string]interface{}{
		"post":     post,
		"author":   author,
		"category": category,
	}

	return successResponse(http.StatusOK, response), nil
}

// createPost creates a new blog post
func (h *PostHandler) createPost(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Check authorization
	authorID := getAuthorID(request)
	if authorID == "" {
		return errorResponse(http.StatusUnauthorized, "Authorization required"), nil
	}

	// Parse request
	var req struct {
		Title         string            `json:"title"`
		Content       string            `json:"content"`
		Excerpt       string            `json:"excerpt"`
		CategoryID    string            `json:"category_id"`
		Tags          []string          `json:"tags"`
		Status        string            `json:"status"`
		FeaturedImage string            `json:"featured_image"`
		Metadata      map[string]string `json:"metadata"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Validate
	if req.Title == "" || req.Content == "" {
		return errorResponse(http.StatusBadRequest, "Title and content are required"), nil
	}

	// Generate slug
	slug := generateSlug(req.Title)

	// Check if slug exists
	var existing models.Post
	err := h.db.Model(&models.Post{}).
		Index("gsi-slug").
		Where("Slug", "=", slug).
		First(&existing)

	if err == nil {
		// Slug exists, append number
		for i := 2; i < 100; i++ {
			testSlug := fmt.Sprintf("%s-%d", slug, i)
			err = h.db.Model(&models.Post{}).
				Index("gsi-slug").
				Where("Slug", "=", testSlug).
				First(&existing)
			if err == dynamorm.ErrNotFound {
				slug = testSlug
				break
			}
		}
	}

	// Create post
	post := &models.Post{
		ID:            uuid.New().String(),
		Slug:          slug,
		AuthorID:      authorID,
		Title:         req.Title,
		Content:       req.Content,
		Excerpt:       req.Excerpt,
		CategoryID:    req.CategoryID,
		Tags:          req.Tags,
		Status:        req.Status,
		FeaturedImage: req.FeaturedImage,
		Metadata:      req.Metadata,
		ViewCount:     0,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}

	if post.Status == "" {
		post.Status = models.PostStatusDraft
	}

	if post.Status == models.PostStatusPublished {
		post.PublishedAt = time.Now()
	}

	// Start transaction
	tx := h.db.Transaction()

	// Create post
	if err := tx.Model(post).Create(); err != nil {
		tx.Rollback()
		return errorResponse(http.StatusInternalServerError, "Failed to create post"), nil
	}

	// Update author post count
	if err := tx.Model(&models.Author{}).
		Where("ID", "=", authorID).
		Increment("PostCount", 1); err != nil {
		tx.Rollback()
		return errorResponse(http.StatusInternalServerError, "Failed to update author"), nil
	}

	// Update category post count
	if req.CategoryID != "" {
		if err := tx.Model(&models.Category{}).
			Where("ID", "=", req.CategoryID).
			Increment("PostCount", 1); err != nil {
			tx.Rollback()
			return errorResponse(http.StatusInternalServerError, "Failed to update category"), nil
		}
	}

	// Create search index
	if post.Status == models.PostStatusPublished {
		searchIndex := &models.SearchIndex{
			ID:          fmt.Sprintf("post-%s", post.ID),
			ContentType: "post",
			SearchTerms: strings.ToLower(fmt.Sprintf("%s %s", post.Title, strings.Join(post.Tags, " "))),
			PostID:      post.ID,
			Title:       post.Title,
			Excerpt:     post.Excerpt,
			Tags:        post.Tags,
			UpdatedAt:   time.Now(),
		}
		if err := tx.Model(searchIndex).Create(); err != nil {
			// Non-critical, don't rollback
			fmt.Printf("Failed to create search index: %v\n", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to commit transaction"), nil
	}

	return successResponse(http.StatusCreated, post), nil
}

// updatePost updates an existing post
func (h *PostHandler) updatePost(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	postID := request.PathParameters["id"]
	if postID == "" {
		return errorResponse(http.StatusBadRequest, "Post ID is required"), nil
	}

	// Check authorization
	authorID := getAuthorID(request)
	if authorID == "" {
		return errorResponse(http.StatusUnauthorized, "Authorization required"), nil
	}

	// Get existing post
	var post models.Post
	err := h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		First(&post)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			return errorResponse(http.StatusNotFound, "Post not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch post"), nil
	}

	// Check permission
	if post.AuthorID != authorID && !isAdmin(request) {
		return errorResponse(http.StatusForbidden, "Permission denied"), nil
	}

	// Parse update request
	var req struct {
		Title         string            `json:"title"`
		Content       string            `json:"content"`
		Excerpt       string            `json:"excerpt"`
		CategoryID    string            `json:"category_id"`
		Tags          []string          `json:"tags"`
		Status        string            `json:"status"`
		FeaturedImage string            `json:"featured_image"`
		Metadata      map[string]string `json:"metadata"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Build updates
	updates := map[string]interface{}{
		"UpdatedAt": time.Now(),
	}

	if req.Title != "" && req.Title != post.Title {
		updates["Title"] = req.Title
		// Update slug if title changed
		newSlug := generateSlug(req.Title)
		if newSlug != post.Slug {
			// Check if new slug is available
			var existing models.Post
			err = h.db.Model(&models.Post{}).
				Index("gsi-slug").
				Where("Slug", "=", newSlug).
				First(&existing)
			if err == dynamorm.ErrNotFound {
				updates["Slug"] = newSlug
			}
		}
	}

	if req.Content != "" {
		updates["Content"] = req.Content
	}
	if req.Excerpt != "" {
		updates["Excerpt"] = req.Excerpt
	}
	if req.CategoryID != post.CategoryID {
		updates["CategoryID"] = req.CategoryID
	}
	if len(req.Tags) > 0 {
		updates["Tags"] = req.Tags
	}
	if req.Status != "" && req.Status != post.Status {
		updates["Status"] = req.Status
		if req.Status == models.PostStatusPublished && post.Status != models.PostStatusPublished {
			updates["PublishedAt"] = time.Now()
		}
	}
	if req.FeaturedImage != "" {
		updates["FeaturedImage"] = req.FeaturedImage
	}
	if req.Metadata != nil {
		updates["Metadata"] = req.Metadata
	}

	// Update post with optimistic locking
	err = h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		Where("Version", "=", post.Version).
		Update(updates)

	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Post was modified by another user"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to update post"), nil
	}

	// Update search index if published
	if req.Status == models.PostStatusPublished || post.Status == models.PostStatusPublished {
		searchIndex := &models.SearchIndex{
			ID:          fmt.Sprintf("post-%s", post.ID),
			ContentType: "post",
			SearchTerms: strings.ToLower(fmt.Sprintf("%s %s", req.Title, strings.Join(req.Tags, " "))),
			PostID:      post.ID,
			Title:       req.Title,
			Excerpt:     req.Excerpt,
			Tags:        req.Tags,
			UpdatedAt:   time.Now(),
		}
		_ = h.db.Model(searchIndex).Create() // Update or create
	}

	// Return updated post
	post.UpdatedAt = updates["UpdatedAt"].(time.Time)
	for k, v := range updates {
		switch k {
		case "Title":
			post.Title = v.(string)
		case "Content":
			post.Content = v.(string)
		case "Status":
			post.Status = v.(string)
		}
	}

	return successResponse(http.StatusOK, post), nil
}

// deletePost deletes a blog post
func (h *PostHandler) deletePost(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	postID := request.PathParameters["id"]
	if postID == "" {
		return errorResponse(http.StatusBadRequest, "Post ID is required"), nil
	}

	// Check authorization
	authorID := getAuthorID(request)
	if authorID == "" {
		return errorResponse(http.StatusUnauthorized, "Authorization required"), nil
	}

	// Get post
	var post models.Post
	err := h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		First(&post)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			return errorResponse(http.StatusNotFound, "Post not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch post"), nil
	}

	// Check permission
	if post.AuthorID != authorID && !isAdmin(request) {
		return errorResponse(http.StatusForbidden, "Permission denied"), nil
	}

	// Soft delete by updating status
	err = h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		Update(map[string]interface{}{
			"Status":    models.PostStatusArchived,
			"UpdatedAt": time.Now(),
		})

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to delete post"), nil
	}

	// Delete search index
	_ = h.db.Model(&models.SearchIndex{}).
		Where("ID", "=", fmt.Sprintf("post-%s", postID)).
		Delete()

	return successResponse(http.StatusOK, map[string]string{
		"message": "Post deleted successfully",
	}), nil
}

// Helper functions

func (h *PostHandler) incrementViewCount(postID, sessionID string) {
	// Track unique views using session
	if sessionID != "" {
		var session models.Session
		err := h.db.Model(&models.Session{}).
			Where("ID", "=", sessionID).
			First(&session)

		if err == nil {
			// Check if already viewed
			for _, viewedID := range session.PostsViewed {
				if viewedID == postID {
					return // Already viewed
				}
			}
		}
	}

	// Increment view count
	_ = h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		Increment("ViewCount", 1)

	// Track view for analytics
	view := &models.PostView{
		ID:        fmt.Sprintf("%s:%d", postID, time.Now().UnixNano()),
		PostID:    postID,
		Timestamp: time.Now(),
		SessionID: sessionID,
		TTL:       time.Now().Add(90 * 24 * time.Hour),
	}
	_ = h.db.Model(view).Create()

	// Update session if exists
	if sessionID != "" {
		_ = h.db.Model(&models.Session{}).
			Where("ID", "=", sessionID).
			Append("PostsViewed", postID)
	}
}

func generateSlug(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)

	// Replace non-alphanumeric characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Limit length
	if len(slug) > 100 {
		slug = slug[:100]
	}

	return slug
}

func getAuthorID(request events.APIGatewayProxyRequest) string {
	// Extract from JWT claims or headers
	// This is a simplified version
	return request.Headers["X-Author-ID"]
}

func getSessionID(request events.APIGatewayProxyRequest) string {
	return request.Headers["X-Session-ID"]
}

func isAuthorized(request events.APIGatewayProxyRequest) bool {
	return getAuthorID(request) != ""
}

func isAdmin(request events.APIGatewayProxyRequest) bool {
	return request.Headers["X-Author-Role"] == models.RoleAdmin
}

func successResponse(statusCode int, data interface{}) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"data":    data,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(body),
	}
}

func errorResponse(statusCode int, message string) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"error":   message,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(body),
	}
}

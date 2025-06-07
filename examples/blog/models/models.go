package models

import (
	"time"
)

// Post represents a blog post
type Post struct {
	ID            string            `dynamorm:"pk" json:"id"`
	Slug          string            `dynamorm:"index:gsi-slug,unique" json:"slug"`
	AuthorID      string            `dynamorm:"index:gsi-author,pk" json:"author_id"`
	Title         string            `json:"title"`
	Content       string            `json:"content"`
	Excerpt       string            `json:"excerpt"`
	Status        string            `dynamorm:"index:gsi-status-date,pk" json:"status"` // draft, published, archived
	PublishedAt   time.Time         `dynamorm:"index:gsi-status-date,sk" json:"published_at"`
	CategoryID    string            `dynamorm:"index:gsi-category" json:"category_id"`
	Tags          []string          `dynamorm:"set" json:"tags"`
	FeaturedImage string            `json:"featured_image,omitempty"`
	ViewCount     int               `json:"view_count"`
	Metadata      map[string]string `dynamorm:"json" json:"metadata,omitempty"`
	CreatedAt     time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt     time.Time         `dynamorm:"updated_at" json:"updated_at"`
	Version       int               `dynamorm:"version" json:"version"`
}

// PostStatus constants
const (
	PostStatusDraft     = "draft"
	PostStatusPublished = "published"
	PostStatusArchived  = "archived"
)

// Comment represents a comment on a post
type Comment struct {
	ID          string    `dynamorm:"pk" json:"id"`
	PostID      string    `dynamorm:"index:gsi-post,pk" json:"post_id"`
	ParentID    string    `dynamorm:"index:gsi-post,sk,prefix:parent" json:"parent_id,omitempty"`
	AuthorID    string    `json:"author_id"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	Content     string    `json:"content"`
	Status      string    `json:"status"` // approved, pending, spam
	IPAddress   string    `json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt   time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// CommentStatus constants
const (
	CommentStatusApproved = "approved"
	CommentStatusPending  = "pending"
	CommentStatusSpam     = "spam"
)

// Author represents a blog author
type Author struct {
	ID          string    `dynamorm:"pk" json:"id"`
	Email       string    `dynamorm:"index:gsi-email,unique" json:"email"`
	Username    string    `dynamorm:"index:gsi-username,unique" json:"username"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio,omitempty"`
	Avatar      string    `json:"avatar,omitempty"`
	Role        string    `json:"role"` // admin, editor, author
	Active      bool      `json:"active"`
	PostCount   int       `json:"post_count"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt   time.Time `dynamorm:"updated_at" json:"updated_at"`
	Version     int       `dynamorm:"version" json:"version"`
}

// AuthorRole constants
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleAuthor = "author"
)

// Category represents a blog category
type Category struct {
	ID          string    `dynamorm:"pk" json:"id"`
	Slug        string    `dynamorm:"index:gsi-slug,unique" json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ParentID    string    `json:"parent_id,omitempty"`
	PostCount   int       `json:"post_count"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt   time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// Tag represents a blog tag
type Tag struct {
	ID        string    `dynamorm:"pk" json:"id"`
	Name      string    `dynamorm:"index:gsi-name,unique" json:"name"`
	Slug      string    `dynamorm:"index:gsi-slug,unique" json:"slug"`
	PostCount int       `json:"post_count"`
	CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
}

// Subscriber represents an email subscriber
type Subscriber struct {
	ID               string    `dynamorm:"pk" json:"id"`
	Email            string    `dynamorm:"index:gsi-email,unique" json:"email"`
	Name             string    `json:"name,omitempty"`
	Status           string    `json:"status"` // active, unsubscribed, bounced
	Categories       []string  `dynamorm:"set" json:"categories,omitempty"`
	Tags             []string  `dynamorm:"set" json:"tags,omitempty"`
	VerifiedAt       time.Time `json:"verified_at,omitempty"`
	UnsubscribeToken string    `json:"-"`
	CreatedAt        time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt        time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// SearchIndex represents a search index entry for full-text search
type SearchIndex struct {
	ID          string    `dynamorm:"pk" json:"id"`                            // post-{postID}
	ContentType string    `dynamorm:"index:gsi-search,pk" json:"content_type"` // post, page
	SearchTerms string    `dynamorm:"index:gsi-search,sk" json:"search_terms"` // lowercase, space-separated
	PostID      string    `json:"post_id"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	Tags        []string  `dynamorm:"set" json:"tags"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Analytics represents page view analytics
type Analytics struct {
	ID          string         `dynamorm:"pk,composite:post_id,date" json:"id"`
	PostID      string         `dynamorm:"extract:post_id" json:"post_id"`
	Date        string         `dynamorm:"extract:date" json:"date"` // YYYY-MM-DD
	Views       int            `json:"views"`
	UniqueViews int            `json:"unique_views"`
	Countries   map[string]int `dynamorm:"json" json:"countries"`
	Referrers   map[string]int `dynamorm:"json" json:"referrers"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Session represents a user session for tracking unique views
type Session struct {
	ID          string    `dynamorm:"pk" json:"id"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	PostsViewed []string  `dynamorm:"set" json:"posts_viewed"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `dynamorm:"ttl" json:"expires_at"`
}

// PostView tracks individual post views for analytics
type PostView struct {
	ID        string    `dynamorm:"pk,composite:post_id,timestamp" json:"id"`
	PostID    string    `dynamorm:"extract:post_id" json:"post_id"`
	Timestamp time.Time `dynamorm:"extract:timestamp" json:"timestamp"`
	SessionID string    `json:"session_id"`
	Country   string    `json:"country,omitempty"`
	Referrer  string    `json:"referrer,omitempty"`
	TTL       time.Time `dynamorm:"ttl" json:"ttl"` // 90 days retention
}

// RelatedPost represents a many-to-many relationship between posts
type RelatedPost struct {
	ID        string    `dynamorm:"pk,composite:post_id,related_id" json:"id"`
	PostID    string    `dynamorm:"extract:post_id" json:"post_id"`
	RelatedID string    `dynamorm:"extract:related_id" json:"related_id"`
	Score     float64   `json:"score"` // Relevance score
	CreatedAt time.Time `json:"created_at"`
}

// ContentBlock represents reusable content blocks
type ContentBlock struct {
	ID        string            `dynamorm:"pk" json:"id"`
	Name      string            `dynamorm:"index:gsi-name,unique" json:"name"`
	Type      string            `json:"type"` // html, markdown, component
	Content   string            `json:"content"`
	Variables map[string]string `dynamorm:"json" json:"variables,omitempty"`
	CreatedAt time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt time.Time         `dynamorm:"updated_at" json:"updated_at"`
	Version   int               `dynamorm:"version" json:"version"`
}

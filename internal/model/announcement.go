package model

import "time"

type AnnouncementAudience string

const (
	AnnouncementAudienceAuthenticated AnnouncementAudience = "authenticated"
	AnnouncementAudienceNewUser       AnnouncementAudience = "new_user"
	AnnouncementAudiencePublic        AnnouncementAudience = "public"
)

type Announcement struct {
	ID        string               `json:"id"`
	Title     string               `json:"title"`
	Content   string               `json:"content"`
	Audience  AnnouncementAudience `json:"audience"`
	Pinned    bool                 `json:"pinned"`
	Enabled   bool                 `json:"enabled"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

type AnnouncementRead struct {
	AnnouncementID string    `json:"announcementId"`
	UserID         string    `json:"userId"`
	ReadAt         time.Time `json:"readAt"`
}

type AnnouncementRequest struct {
	Title    string               `json:"title" binding:"required,min=1,max=120"`
	Content  string               `json:"content" binding:"required,min=1,max=10000"`
	Audience AnnouncementAudience `json:"audience" binding:"required,oneof=authenticated new_user public"`
	Pinned   bool                 `json:"pinned"`
	Enabled  bool                 `json:"enabled"`
}

type AnnouncementResponse struct {
	ID        string               `json:"id"`
	Title     string               `json:"title"`
	Content   string               `json:"content"`
	Audience  AnnouncementAudience `json:"audience"`
	Pinned    bool                 `json:"pinned"`
	Enabled   bool                 `json:"enabled"`
	IsRead    bool                 `json:"isRead"`
	ReadAt    *time.Time           `json:"readAt,omitempty"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

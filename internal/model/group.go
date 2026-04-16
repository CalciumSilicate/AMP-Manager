package model

import (
	"time"

	"ampmanager/internal/precision"
)

type Group struct {
	ID                string    `json:"id"`
	RateMultiplierPPM int64     `json:"rateMultiplierPpm,omitempty"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	RateMultiplier    float64   `json:"rateMultiplier"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type GroupRequest struct {
	Name              string                  `json:"name" binding:"required,min=1,max=64"`
	Description       string                  `json:"description" binding:"max=256"`
	RateMultiplier    precision.DecimalString `json:"rateMultiplier"`
	RateMultiplierPPM *int64                  `json:"rateMultiplierPpm,omitempty"`
}

type GroupResponse struct {
	ID                string    `json:"id"`
	RateMultiplierPPM int64     `json:"rateMultiplierPpm,omitempty"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	RateMultiplier    float64   `json:"rateMultiplier"`
	UserCount         int       `json:"userCount"`
	ChannelCount      int       `json:"channelCount"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

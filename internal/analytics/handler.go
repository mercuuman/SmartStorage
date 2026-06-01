package analytics

import "github.com/gin-gonic/gin"

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}
func (h *Handler) GetSystemStats(c *gin.Context) {

	stats, err := h.service.GetSystemStats(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, stats)
}
func (h *Handler) GetUserStats(c *gin.Context) {

	userID := c.GetString("userID") // из JWT middleware

	stats, err := h.service.GetUserStats(c.Request.Context(), userID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, stats)
}
func (h *Handler) GetCompressionStats(c *gin.Context) {

	stats, err := h.service.GetCompressionStats(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, stats)
}

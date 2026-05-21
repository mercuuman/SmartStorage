package files

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Upload(c *gin.Context) {
	userID := c.GetString("userID")
	orgID := c.GetString("organizationID")

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open file"})
		return
	}
	defer src.Close()

	file, err := h.service.Upload(c.Request.Context(), userID, orgID, fileHeader.Filename, src)
	if err != nil {
		log.Printf("upload failed: userID=%s filename=%s err=%v", userID, fileHeader.Filename, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload file"})
		return
	}

	c.JSON(http.StatusCreated, file)
}

func (h *Handler) List(c *gin.Context) {
	userID := c.GetString("userID")

	files, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get files"})
		return
	}

	c.JSON(http.StatusOK, files)
}

func (h *Handler) Download(c *gin.Context) {
	id := c.Param("id")

	file, err := h.service.GetDownloadFile(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	c.FileAttachment(file.StoragePath, file.Filename)
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		log.Printf("delete failed: id=%s err=%v", id, err) // ← добавьте это
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "file deleted"})
}

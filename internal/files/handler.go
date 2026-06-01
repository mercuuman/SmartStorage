package files

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

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

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "file is required",
		})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to open file",
		})
		return
	}

	defer src.Close()

	file, err := h.service.Upload(
		c.Request.Context(),
		userID,
		fileHeader.Filename,
		src,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
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

	reader, filename, err :=
		h.service.Download(
			c.Request.Context(),
			id,
		)

	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "file not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	defer reader.Close()

	c.Header(
		"Content-Disposition",
		`attachment; filename="`+filename+`"`,
	)

	c.Header(
		"Content-Type",
		"application/octet-stream",
	)

	_, err = io.Copy(
		c.Writer,
		reader,
	)

	if err != nil {
		return
	}
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

func (h *Handler) GetVersionHistory(c *gin.Context) {

	fileID := c.Param("id")

	versions, err := h.service.GetVersionHistory(
		c.Request.Context(),
		fileID,
	)

	if err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})

		return
	}

	c.JSON(http.StatusOK, versions)
}
func (h *Handler) RestoreVersion(c *gin.Context) {

	fileID := c.Param("id")

	versionNumber, err := strconv.Atoi(
		c.Param("version"),
	)

	if err != nil {

		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid version",
		})

		return
	}

	err = h.service.RestoreVersion(
		c.Request.Context(),
		fileID,
		versionNumber,
	)

	if err != nil {

		if errors.Is(err, ErrNotFound) {

			c.JSON(http.StatusNotFound, gin.H{
				"error": "version not found",
			})

			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "version restored",
	})
}
func (h *Handler) GetFileDetails(c *gin.Context) {
	fileID := c.Param("id")
	userID := c.GetString("userID")
	details, err := h.service.GetFileDetails(c.Request.Context(), fileID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		log.Printf("get file details failed: id=%s err=%v", fileID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get file details"})
		return
	}
	if details.File.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	c.JSON(http.StatusOK, details)
}

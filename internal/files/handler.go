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
	folderID := c.PostForm("folder_id")
	var fid *string
	if folderID != "" {
		fid = &folderID
	}

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

	file, err := h.service.Upload(c.Request.Context(), userID, fileHeader.Filename, src, fid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, file)
}

func (h *Handler) List(c *gin.Context) {
	userID := c.GetString("userID")
	folderID := c.Query("folder_id")
	var fid *string
	if folderID != "" {
		fid = &folderID
	}
	files, err := h.service.List(c.Request.Context(), userID, fid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get files"})
		return
	}
	c.JSON(http.StatusOK, files)
}

func (h *Handler) Download(c *gin.Context) {
	id := c.Param("id")
	reader, filename, err := h.service.Download(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer reader.Close()
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Content-Type", "application/octet-stream")
	_, _ = io.Copy(c.Writer, reader)
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		log.Printf("delete failed: id=%s err=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to move to trash"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "file moved to trash"})
}

func (h *Handler) GetVersionHistory(c *gin.Context) {
	fileID := c.Param("id")
	versions, err := h.service.GetVersionHistory(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, versions)
}

func (h *Handler) RestoreVersion(c *gin.Context) {
	fileID := c.Param("id")
	versionNumber, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version"})
		return
	}
	err = h.service.RestoreVersion(c.Request.Context(), fileID, versionNumber)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "version restored"})
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

// ==================== FOLDER HANDLERS ====================

func (h *Handler) CreateFolder(c *gin.Context) {
	userID := c.GetString("userID")
	var req struct {
		Name     string  `json:"name" binding:"required"`
		ParentID *string `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	folder, err := h.service.CreateFolder(c.Request.Context(), userID, req.Name, req.ParentID)
	if err != nil {
		if err.Error() == "folder with this name already exists" {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, folder)
}

// ListFolders — гарантируем, что всегда возвращаем массив
func (h *Handler) ListFolders(c *gin.Context) {
	userID := c.GetString("userID")
	parentID := c.Query("parent_id")

	var pid *string
	if parentID != "" {
		pid = &parentID
	}

	folders, err := h.service.GetFolders(c.Request.Context(), userID, pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list folders"})
		return
	}

	// 🔑 ФИКС: если nil — возвращаем пустой массив
	if folders == nil {
		folders = []Folder{}
	}

	c.JSON(http.StatusOK, folders)
}

func (h *Handler) GetFolder(c *gin.Context) {
	userID := c.GetString("userID")
	folderID := c.Param("id")
	contents, err := h.service.GetFolderContents(c.Request.Context(), userID, folderID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	c.JSON(http.StatusOK, contents)
}

func (h *Handler) DeleteFolder(c *gin.Context) {
	userID := c.GetString("userID")
	folderID := c.Param("id")
	if err := h.service.DeleteFolder(c.Request.Context(), userID, folderID); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "folder moved to trash"})
}

// ==================== TRASH HANDLERS ====================

func (h *Handler) GetTrashContents(c *gin.Context) {
	userID := c.GetString("userID")
	tx, _ := h.service.repo.BeginTx(c.Request.Context())
	files, err := h.service.repo.GetTrashFiles(c.Request.Context(), tx, userID)
	_ = tx.Rollback(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load trash"})
		return
	}
	c.JSON(http.StatusOK, files)
}

func (h *Handler) EmptyTrash(c *gin.Context) {
	userID := c.GetString("userID")
	if err := h.service.EmptyTrash(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "trash emptied"})
}

func (h *Handler) RestoreFromTrash(c *gin.Context) {
	userID := c.GetString("userID")
	var req struct {
		FileID         *string `json:"file_id"`
		FolderID       *string `json:"folder_id"`
		TargetParentID *string `json:"target_parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.FileID == nil && req.FolderID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_id or folder_id required"})
		return
	}
	if err := h.service.RestoreFromTrash(c.Request.Context(), userID, req.FileID, req.FolderID, req.TargetParentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "restored from trash"})
}

package files

import "time"

type File struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	OrganizationID   *string   `json:"organization_id,omitempty"`
	Filename         string    `json:"filename"`
	CurrentVersionID *string   `json:"current_version_id,omitempty"`
	IsDeleted        bool      `json:"is_deleted"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type FileVersion struct {
	ID             string    `json:"id"`
	FileID         string    `json:"file_id"`
	PhysicalFileID string    `json:"physical_file_id"`
	VersionNumber  int       `json:"version_number"`
	UploadedBy     *string   `json:"uploaded_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type PhysicalFile struct {
	ID                   string    `json:"id"`
	HashSHA256           string    `json:"hash_sha256"`
	StoragePath          string    `json:"storage_path"`
	OriginalSize         int64     `json:"original_size"`
	CompressedSize       *int64    `json:"compressed_size,omitempty"`
	CompressionAlgorithm *string   `json:"compression_algorithm,omitempty"`
	CompressionRatio     *float64  `json:"compression_ratio,omitempty"`
	ReferenceCount       int       `json:"reference_count"`
	CreatedAt            time.Time `json:"created_at"`
}

type FileListItem struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	OrganizationID   *string   `json:"organization_id,omitempty"`
	Filename         string    `json:"filename"`
	CurrentVersionID *string   `json:"current_version_id,omitempty"`
	IsDeleted        bool      `json:"is_deleted"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type DownloadFile struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	StoragePath string `json:"storage_path"`
}

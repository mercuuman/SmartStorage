package files

import "time"

type File struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	OrganizationID   *string   `json:"organization_id,omitempty"`
	Filename         string    `json:"filename"`
	FolderID         *string   `json:"folder_id,omitempty"`
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
	UploadedBy     string    `json:"uploaded_by,omitempty"`
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
	FolderID         *string   `json:"folder_id,omitempty"`
	CurrentVersionID *string   `json:"current_version_id,omitempty"`
	IsDeleted        bool      `json:"is_deleted"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	SizeBytes        *int64    `json:"size_bytes,omitempty"`
}

type DownloadFile struct {
	ID                   string  `json:"id"`
	Filename             string  `json:"filename"`
	StoragePath          string  `json:"storage_path"`
	CompressionAlgorithm *string `json:"compression_algorithm"`
}

type FileVersionInfo struct {
	ID            string    `json:"id"`
	VersionNumber int       `json:"version_number"`
	CreatedAt     time.Time `json:"created_at"`

	PhysicalFileID string `json:"physical_file_id"`
	OriginalSize   int64  `json:"original_size"`

	CompressedSize       *int64   `json:"compressed_size"`
	CompressionAlgorithm *string  `json:"compression_algorithm"`
	CompressionRatio     *float64 `json:"compression_ratio"`
}

type FileDetails struct {
	File           File            `json:"file"`
	CurrentVersion VersionDetails  `json:"current_version"`
	PhysicalFile   PhysicalDetails `json:"physical_file"`
}

type VersionDetails struct {
	ID         string    `json:"id,omitempty"`
	Number     int       `json:"version_number"`
	UploadedBy string    `json:"uploaded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	HasData    bool      `json:"-"`
}

type PhysicalDetails struct {
	ID                   string    `json:"id,omitempty"`
	HashSHA256           string    `json:"hash_sha256,omitempty"`
	StoragePath          string    `json:"storage_path,omitempty"`
	OriginalSize         int64     `json:"original_size"`
	CompressedSize       *int64    `json:"compressed_size,omitempty"`
	CompressionAlgorithm *string   `json:"compression_algorithm,omitempty"`
	CompressionRatio     *float64  `json:"compression_ratio,omitempty"`
	ReferenceCount       int       `json:"reference_count"`
	CreatedAt            time.Time `json:"created_at,omitempty"`
	HasData              bool      `json:"-"`
}

type Folder struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ParentID  *string   `json:"parent_id,omitempty"`
	Name      string    `json:"name"`
	IsSystem  bool      `json:"is_system"`
	CreatedAt time.Time `json:"created_at"`
}

type FolderContents struct {
	Folder     Folder         `json:"folder"`
	Subfolders []Folder       `json:"subfolders"`
	Files      []FileListItem `json:"files"`
}

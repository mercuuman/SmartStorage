package analytics

type SystemStats struct {
	TotalFiles         int64 `json:"total_files"`
	TotalVersions      int64 `json:"total_versions"`
	TotalPhysicalFiles int64 `json:"total_physical_files"`

	TotalOriginalSize   int64 `json:"total_original_size"`
	TotalCompressedSize int64 `json:"total_compressed_size"`

	SpaceSavedBytes   int64   `json:"space_saved_bytes"`
	SpaceSavedPercent float64 `json:"space_saved_percent"`
}

type UserStats struct {
	UserID string `json:"user_id"`

	TotalFiles    int64 `json:"total_files"`
	TotalVersions int64 `json:"total_versions"`

	TotalOriginalSize   int64 `json:"total_original_size"`
	TotalCompressedSize int64 `json:"total_compressed_size"`

	SpaceSavedBytes   int64   `json:"space_saved_bytes"`
	SpaceSavedPercent float64 `json:"space_saved_percent"`

	TotalDeduplicatedFiles int64 `json:"total_deduplicated_files"`
}
type CompressionStats struct {
	Algorithm string `json:"algorithm"`

	FilesCount int64 `json:"files_count"`

	TotalOriginalSize   int64 `json:"total_original_size"`
	TotalCompressedSize int64 `json:"total_compressed_size"`

	AvgCompressionRatio float64 `json:"avg_compression_ratio"`
}

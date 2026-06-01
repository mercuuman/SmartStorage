package analytics

import "context"

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetSystemStats(ctx context.Context) (*SystemStats, error) {
	return s.repo.GetSystemStats(ctx)
}
func (s *Service) GetUserStats(ctx context.Context, userID string) (*UserStats, error) {
	return s.repo.GetUserStats(ctx, userID)
}
func (s *Service) GetCompressionStats(ctx context.Context) ([]CompressionStats, error) {
	return s.repo.GetCompressionStats(ctx)
}

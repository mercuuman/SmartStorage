package garbage

import (
	"diplom/internal/files"
)

type Service struct {
	repo *files.Repository
}

func NewService(repo *files.Repository) *Service {
	return &Service{repo: repo}
}

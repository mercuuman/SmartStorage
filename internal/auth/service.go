package auth

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{
		repo: repo,
	}
}

func (s *Service) Register(req RegisterRequest) error {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(req.Password),
		bcrypt.DefaultCost,
	)

	if err != nil {
		return err
	}

	return s.repo.CreateUser(
		req.Email,
		string(hash),
	)
}

func (s *Service) Login(req LoginRequest) (string, error) {
	user, err := s.repo.GetUserByEmail(req.Email)
	if err != nil {
		return "", err
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(req.Password),
	)

	if err != nil {
		return "", errors.New("invalid credentials")
	}

	token, err := GenerateToken(user.ID)
	if err != nil {
		return "", err
	}

	return token, nil
}
func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

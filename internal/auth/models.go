package auth

import "time"

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"` // 🔥 Добавили имя при регистрации
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// 🔥 Расширенная модель пользователя
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // 🔥 Не отдаём в JSON!
	Name         *string   `json:"name"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
